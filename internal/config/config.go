package config

import (
	"fmt"
	"os"
	"time"
)

func getConfigFromEnv(key string) (string, error) {
	if os.Getenv(key) != "" {
		return os.Getenv(key), nil
	}
	return "", fmt.Errorf("Cannot find %s from from environment variable", key)
}

func GetSyncPeriod() time.Duration {
	defaultSyncPeriod := time.Second * 150
	syncPeriodStr, err := getConfigFromEnv("SYNC_PERIOD")
	if err != nil {
		return defaultSyncPeriod
	}
	syncPeriod, err := time.ParseDuration(syncPeriodStr)
	if err != nil {
		return defaultSyncPeriod
	}
	return syncPeriod
}
