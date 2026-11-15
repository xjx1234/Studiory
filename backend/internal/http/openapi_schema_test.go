package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"backend/internal/openapi"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestOpenAPISchemaMatchesHandlers 把契约检查从「方法 + 路径」推进到「字段级」：
//
//  1. 文档 requestBody schema 的属性名，必须与 Go 请求结构体的 json tag 完全一致
//     （这正是 README 把 type 写成 code_type 这类漂移的检测点）；
//  2. 文档认可的代表性请求，必须能通过真实路由，且不被 binding 判为 20001/20002；
//  3. 成功响应的 JSON 必须能通过文档里 200 响应的 schema 校验。
//
// 只覆盖三个高频入口；新增字段时文档与实现的任何一侧漏改都会让这里失败。
func TestOpenAPISchemaMatchesHandlers(t *testing.T) {
	doc, err := openapi.LoadDocument(openapiSpecPath(t))
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}

	r := newContractRouter(t)

	cases := []struct {
		name          string
		path          string
		requestType   reflect.Type
		payloads      []map[string]any
		checkResponse bool
	}{
		{
			name:        "register",
			path:        "/api/v1/auth/register",
			requestType: reflect.TypeOf(RegisterReq{}),
			payloads: []map[string]any{
				{"grant_type": "password", "phone": "13800138000", "code": "123456", "password": "Str0ng!Pass"},
				{"grant_type": "code", "code_type": "sms", "phone": "13800138000", "code": "123456"},
			},
			checkResponse: true,
		},
		{
			name:        "login",
			path:        "/api/v1/auth/login",
			requestType: reflect.TypeOf(LoginReq{}),
			payloads: []map[string]any{
				{"grant_type": "password", "phone": "13800138000", "password": "Str0ng!Pass"},
				{"grant_type": "sms_code", "phone": "13800138000", "code": "123456"},
			},
			checkResponse: true,
		},
		{
			name:        "send-code",
			path:        "/api/v1/auth/send-code",
			requestType: reflect.TypeOf(SendCodeReq{}),
			payloads: []map[string]any{
				{"type": "sms", "target": "13800138000"},
				{"type": "email", "target": "dev@example.com"},
			},
			checkResponse: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := doc.Paths.Find(tc.path)
			if item == nil || item.Post == nil {
				t.Fatalf("openapi is missing POST %s", tc.path)
			}
			op := item.Post

			if op.RequestBody == nil || op.RequestBody.Value == nil {
				t.Fatalf("%s has no requestBody", tc.path)
			}
			media := op.RequestBody.Value.Content.Get("application/json")
			if media == nil || media.Schema == nil || media.Schema.Value == nil {
				t.Fatalf("%s has no application/json request schema", tc.path)
			}
			requestSchema := media.Schema.Value

			comparePropertyNames(t, requestSchema, tc.requestType)

			for _, payload := range tc.payloads {
				payload := payload
				if err := requestSchema.VisitJSON(payload); err != nil {
					t.Errorf("documented payload %v does not satisfy its own schema: %v", payload, err)
				}

				body, marshalErr := json.Marshal(payload)
				if marshalErr != nil {
					t.Fatalf("marshal payload: %v", marshalErr)
				}
				req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.RemoteAddr = "203.0.113.20:5555"
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					t.Fatalf("payload %v rejected by handler: status = %d, body = %s", payload, w.Code, w.Body.String())
				}
				var envelope struct {
					Code int `json:"code"`
				}
				if unmarshalErr := json.Unmarshal(w.Body.Bytes(), &envelope); unmarshalErr != nil {
					t.Fatalf("unmarshal response (%s): %v", w.Body.String(), unmarshalErr)
				}
				if envelope.Code != 0 {
					t.Fatalf("payload %v hit a request-validation error: body = %s", payload, w.Body.String())
				}

				if tc.checkResponse {
					assertResponseMatchesSchema(t, op, w.Body.Bytes())
				}
			}
		})
	}
}

// comparePropertyNames 要求文档属性名与 Go 结构体 json tag 的集合完全一致。
func comparePropertyNames(t *testing.T, schema *openapi3.Schema, typ reflect.Type) {
	t.Helper()

	docProps := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		docProps = append(docProps, name)
	}

	codeProps := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		codeProps = append(codeProps, name)
	}

	sort.Strings(docProps)
	sort.Strings(codeProps)

	if strings.Join(docProps, ",") != strings.Join(codeProps, ",") {
		t.Fatalf("openapi properties drifted from %s json tags:\n  openapi: %v\n  go:      %v",
			typ.Name(), docProps, codeProps)
	}
}

// assertResponseMatchesSchema 用文档里的 200 schema 校验真实响应体。
func assertResponseMatchesSchema(t *testing.T, op *openapi3.Operation, body []byte) {
	t.Helper()

	resp := op.Responses.Status(http.StatusOK)
	if resp == nil || resp.Value == nil {
		t.Fatalf("openapi is missing 200 response for operation %q", op.OperationID)
	}
	media := resp.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("openapi is missing the 200 application/json schema for operation %q", op.OperationID)
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if err := media.Schema.Value.VisitJSON(decoded); err != nil {
		t.Errorf("response does not match documented schema: %v\nbody: %s", err, body)
	}
}
