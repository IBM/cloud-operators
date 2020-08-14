package config

import (
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
)

var (
	loadOnce sync.Once
	config   Config
)

type Config struct {
	APIKey            string        `envconfig:"bluemix_api_key"`
	AccountID         string        `envconfig:"bluemix_account_id"`
	Org               string        `envconfig:"bluemix_org"`
	Region            string        `envconfig:"bluemix_region"`
	ResourceGroupName string        `envconfig:"bluemix_resource_group"`
	Space             string        `envconfig:"bluemix_space"`
	SyncPeriod        time.Duration `envconfig:"sync_period"`
}

func Get() Config {
	loadOnce.Do(func() {
		config = Config{ // default values
			SyncPeriod: 150 * time.Second,
		}
		envconfig.MustProcess("", &config)
	})
	return config
}
