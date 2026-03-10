package session

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	SessionTTL     time.Duration
	CookieName     string
	CookieDomain   string
	CookiePath     string
	CookieSecure   bool
	CookieHTTPOnly bool
	CookieSameSite string
}

func ConfigFromEnv() Config {
	return Config{
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        getEnvInt("REDIS_DB", 0),
		SessionTTL:     getEnvDuration("SESSION_TTL", 8*time.Hour),
		CookieName:     getEnv("SESSION_COOKIE_NAME", "rc_session"),
		CookieDomain:   strings.TrimSpace(os.Getenv("SESSION_COOKIE_DOMAIN")),
		CookiePath:     getEnv("SESSION_COOKIE_PATH", "/"),
		CookieSecure:   getEnvBool("SESSION_COOKIE_SECURE", false),
		CookieHTTPOnly: getEnvBool("SESSION_COOKIE_HTTP_ONLY", true),
		CookieSameSite: strings.ToLower(getEnv("SESSION_COOKIE_SAME_SITE", "lax")),
	}
}

func getEnv(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}
