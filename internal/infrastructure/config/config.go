package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port               string
	DBDSN              string
	AMQPURL            string
	DispatchIntervalMS int
}

func Load() Config {
	return Config{
		Port:               getEnv("OS_PORT", "8081"),
		DBDSN:              getEnv("OS_DB_DSN", "host=localhost user=postgres password=postgres dbname=os_service port=5432 sslmode=disable"),
		AMQPURL:            getEnv("OS_AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		DispatchIntervalMS: getEnvInt("OS_DISPATCH_INTERVAL_MS", 500),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
