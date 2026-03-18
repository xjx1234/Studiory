package config

import "os"

// Config 从环境变量读取运行配置，后续可改为配置文件或 viper。
type Config struct {
	// 服务
	ServerAddr string
	AppEnv    string

	// 数据库
	DatabaseURL string

	// Redis
	RedisURL string

	// JWT
	JWTSecret string
}

// Load 从环境变量加载配置。
func Load() *Config {
	return &Config{
		ServerAddr:  envOr("SERVER_ADDR", ":8080"),
		AppEnv:      envOr("APP_ENV", "development"),
		DatabaseURL: envOr("DATABASE_URL", "postgres://localhost:5432/shixishe?sslmode=disable"),
		RedisURL:    envOr("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:   envOr("JWT_SECRET", "dev-secret-change-in-production"),
	}
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
