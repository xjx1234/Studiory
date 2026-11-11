package config

import (
	"strings"
	"testing"
)

// TestBuildDatabaseURLMySQLHasUTCParams 验证分项配置拼接出的 MySQL DSN
// 自带 parseTime/loc/time_zone，DATETIME 读写锁定 UTC，不随服务器时区漂移。
func TestBuildDatabaseURLMySQLHasUTCParams(t *testing.T) {
	c := &Config{
		DBDriver:   "mysql",
		DBHost:     "127.0.0.1",
		DBPort:     "3306",
		DBUser:     "app",
		DBPassword: "secret",
		DBName:     "app",
	}
	dsn := c.buildDatabaseURL()
	for _, want := range []string{"parseTime=true", "loc=UTC", "time_zone=%27%2B00%3A00%27"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN missing %q in %s", want, dsn)
		}
	}
}

// TestNormalizeMySQLDSNCoversURIBypass 验证 database.url / DATABASE_URL 旁路
// 拼接公式时，归一化仍会补齐缺失的时区参数，防止偏移 bug 借道复活。
func TestNormalizeMySQLDSNCoversURIBypass(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"bare tcp dsn", "app:secret@tcp(127.0.0.1:3306)/app"},
		{"mysql scheme", "mysql://app:secret@tcp(127.0.0.1:3306)/app?multiStatements=true"},
		{"already normalized is idempotent", "app:secret@tcp(127.0.0.1:3306)/app?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeMySQLDSN(tt.dsn)
			if err != nil {
				t.Fatalf("NormalizeMySQLDSN: %v", err)
			}
			for _, want := range []string{"parseTime=true", "loc=UTC", "time_zone=%27%2B00%3A00%27"} {
				if !strings.Contains(got, want) {
					t.Errorf("normalized DSN missing %q: %s", want, got)
				}
			}
			// multiStatements 等原有参数必须保留
			if strings.Contains(tt.dsn, "multiStatements") && !strings.Contains(got, "multiStatements=true") {
				t.Errorf("original params dropped: %s", got)
			}
		})
	}
}

func TestNormalizeMySQLDSNRejectsGarbage(t *testing.T) {
	if _, err := NormalizeMySQLDSN("not a dsn at all"); err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}
