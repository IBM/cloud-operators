package common

/*
 * Copyright 2019 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

func GetConfigFromEnv(key string) (string, error) {
	if os.Getenv(key) != "" {
		return os.Getenv(key), nil
	} else {
		return "", errors.New(fmt.Sprintf("Cannot find %s from from environment variable", key))
	}
}

func GetSyncPeriod() time.Duration {
	defaultSyncPeriod := time.Second * 150
	syncPeriodStr, err := GetConfigFromEnv("SYNC_PERIOD")
	if err != nil {
		return defaultSyncPeriod
	}
	syncPeriod, err := time.ParseDuration(syncPeriodStr)
	if err != nil {
		return defaultSyncPeriod
	}
	return syncPeriod
}

func GetMaxConcurrentReconciles() int {
	maxConcurrentReconciles := 1
	maxConcurrentReconcilesStr, err := GetConfigFromEnv("MAX_CONCURRENT_RECONCILES")
	if err == nil {
		maxConcurrentReconciles, err = strconv.Atoi(maxConcurrentReconcilesStr)
		if err != nil {
			maxConcurrentReconciles = 1
		}
	}
	return maxConcurrentReconciles
}
