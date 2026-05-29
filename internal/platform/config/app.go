package config

import "os"

func GetEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func AppPort() string {
	return GetEnv("APP_PORT", "8080")
}

func JWTSecret() string {
	return GetEnv("JWT_SECRET", "change-me")
}
