package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

func getConfigFromEnv(key string) (string, error) {
	if os.Getenv(key) != "" {
		return os.Getenv(key), nil
	} else {
		return "", errors.New(fmt.Sprintf("Cannot find %s from from environment variable", key))
	}
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

func getMaxConcurrentReconciles() int {
	maxConcurrentReconciles := 1
	maxConcurrentReconcilesStr, err := getConfigFromEnv("MAX_CONCURRENT_RECONCILES")
	if err == nil {
		maxConcurrentReconciles, err = strconv.Atoi(maxConcurrentReconcilesStr)
		if err != nil {
			maxConcurrentReconciles = 1
		}
	}
	return maxConcurrentReconciles
}
