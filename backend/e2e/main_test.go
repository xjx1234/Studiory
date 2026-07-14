//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"backend/internal/app"
	"backend/internal/config"
	"backend/internal/repo"
	"backend/internal/repo/pg"
	"backend/internal/testutil/integration"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var (
	e2eRouter *gin.Engine
	e2eStore  *pg.Store
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()

	pgEnv, err := integration.StartPostgres(ctx)
	if err != nil {
		log.Fatalf("start postgres: %v", err)
	}
	redisEnv, err := integration.StartRedis(ctx)
	if err != nil {
		pgEnv.Close(ctx)
		log.Fatalf("start redis: %v", err)
	}

	e2eStore = pg.NewStore(pgEnv.Pool)

	cfg := testConfig(pgEnv.DSN, redisEnv.URL)
	logger := zap.NewNop()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "app.New failed: %v\n", err)
		os.Exit(1)
	}
	e2eRouter = a.Router

	code := m.Run()

	a.Close()
	pgEnv.Close(ctx)
	redisEnv.Close(ctx)
	os.Exit(code)
}

func testConfig(databaseURL, redisURL string) *config.Config {
	return &config.Config{
		AppEnv:                  "test",
		ServerAddr:              ":8080",
		ServerReadHeaderTimeout: 5 * time.Second,
		ServerReadTimeout:       15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		DatabaseURL:             databaseURL,
		RedisURL:                redisURL,
		RedisKeyPrefix:          "e2e",
		JWTSecret:               "e2e-test-jwt-secret",
		JWTAccessTokenTTL:       2 * time.Hour,
		JWTRefreshTokenTTL:      168 * time.Hour,
		AuthMockCodeEnabled:     true,
		AuthMultiDeviceEnabled:  true,
		OAuthDevMode:            true,
		OAuthProviders:          []string{"wechat", "apple", "google"},
		LogLevel:                "error",
		LogFormat:               "json",
		RateLimitPerMinute:      1000,
		RateLimitUserPerMinute:  1000,
		MetricsEnabled:          false,
		CORSAllowOrigins:        []string{"http://localhost:5173"},
		CORSAllowCredentials:    true,
	}
}

type apiResponse struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type loginData struct {
	Tokens tokenPair `json:"tokens"`
	User   struct {
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
		Role     string `json:"role"`
	} `json:"user"`
}

func uniquePhone() string {
	// 138 + 8 位数字，满足 phone_cn 且避免 uuid 十六进制字母
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	n := uint64(0)
	for _, b := range buf {
		n = n*256 + uint64(b)
	}
	return fmt.Sprintf("138%08d", n%100000000)
}

func doJSON(t *testing.T, method, path string, body any, headers ...string) (*httptest.ResponseRecorder, apiResponse) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	w := httptest.NewRecorder()
	e2eRouter.ServeHTTP(w, req)

	var parsed apiResponse
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("unmarshal response (%s): %v", w.Body.String(), err)
		}
	}
	return w, parsed
}

func bearer(token string) []string {
	return []string{"Authorization", "Bearer " + token}
}

func registerAndLogin(t *testing.T, phone, password string) loginData {
	t.Helper()

	const strongPass = "Str0ng!Pass"
	if password == "" {
		password = strongPass
	}

	w, body := doJSON(t, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"grant_type": "password",
		"phone":      phone,
		"password":   password,
		"nickname":   "e2e-user",
	})
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("register: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	w, body = doJSON(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"grant_type": "password",
		"phone":      phone,
		"password":   password,
	})
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("login: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	var data loginData
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatalf("decode login data: %v", err)
	}
	if data.Tokens.AccessToken == "" {
		t.Fatal("expected access_token")
	}
	return data
}

func createAdminUser(t *testing.T, phone, password string) *repo.User {
	t.Helper()
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hashStr := string(hash)

	u, err := e2eStore.Users().Create(ctx, &repo.CreateUserParams{
		Phone:        strPtr(phone),
		PasswordHash: &hashStr,
		Nickname:     "e2e-admin",
		Role:         repo.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return u
}

func strPtr(s string) *string { return &s }
