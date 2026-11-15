package http

import "testing"

// TestRegisterReqValidate_IdentifierRules 锁定注册请求的跨字段规则：
// 两种 grant 都必须且只能提供一个标识，显式 code_type 必须与标识一致。
func TestRegisterReqValidate_IdentifierRules(t *testing.T) {
	cases := []struct {
		name    string
		req     RegisterReq
		wantErr bool
	}{
		{
			name: "password with phone",
			req:  RegisterReq{GrantType: "password", Phone: "13800138000", Code: "123456", Password: "Str0ng!Pass"},
		},
		{
			name: "password with email",
			req:  RegisterReq{GrantType: "password", Email: "dev@example.com", Code: "123456", Password: "Str0ng!Pass"},
		},
		{
			name: "password with matching explicit code_type",
			req:  RegisterReq{GrantType: "password", CodeType: "email", Email: "dev@example.com", Code: "123456"},
		},
		{
			name: "code with sms",
			req:  RegisterReq{GrantType: "code", CodeType: "sms", Phone: "13800138000", Code: "123456"},
		},
		{
			name:    "password with two identifiers is rejected",
			req:     RegisterReq{GrantType: "password", Phone: "13800138000", Email: "dev@example.com", Code: "123456"},
			wantErr: true,
		},
		{
			name:    "code with two identifiers is rejected",
			req:     RegisterReq{GrantType: "code", CodeType: "sms", Phone: "13800138000", Email: "dev@example.com", Code: "123456"},
			wantErr: true,
		},
		{
			name:    "no identifier is rejected",
			req:     RegisterReq{GrantType: "password", Code: "123456"},
			wantErr: true,
		},
		{
			name:    "explicit code_type must match the identifier",
			req:     RegisterReq{GrantType: "code", CodeType: "email", Phone: "13800138000", Code: "123456"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
