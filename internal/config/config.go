package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort  string
	RedisAddr   string
	RedisPass   string
	RedisDB     int
	WorkerCount int
}

func Load() *Config {
	return &Config{
		ServerPort:  getEnv("SERVER_PORT", "8080"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:   getEnv("REDIS_PASSWORD", ""),
		RedisDB:     getEnvInt("REDIS_DB", 0),
		WorkerCount: getEnvInt("WORKER_COUNT", 3),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
