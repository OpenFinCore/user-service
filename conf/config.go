package conf

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"ofc-user-service/logger"
)

var configFileName = "config.json"

type Config struct {
	Logger         logger.LoggerConfig     `json:"logger"`
	PostDataSource PostgreDataSourceConfig `json:"postgreDataSource"`
	SeedData       SeedConfig              `json:"seed"`
	Stage          string                  `json:"stage"`
}

// SeedConfig points at the directory containing the seed JSON files.
// Every file in this directory is matched by name (without extension) to a
// registered table/model in service.SeedRegistry.
type SeedConfig struct {
	Dir string `json:"dir"`
}

type PostgreDataSourceConfig struct {
	DriverName        string `json:"driverName"`
	Addr              string `json:"addr"`
	Port              string `json:"port"`
	Database          string `json:"database"`
	User              string `json:"user"`
	Password          string `json:"password"`
	SSLMode           string `json:"sslMode"`
	EnableAutoMigrate bool   `json:"enableAutoMigrate"`
}

var config Config
var configOnce sync.Once

func GetConfig() *Config {
	configOnce.Do(func() {
		path, err := resolveConfigPath()
		if err != nil {
			panic(err)
		}

		bytes, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}

		err = json.Unmarshal(bytes, &config)
		if err != nil {
			panic(err)
		}

		if config.PostDataSource.SSLMode == "" {
			config.PostDataSource.SSLMode = "disable"
		}
	})
	return &config
}

func resolveConfigPath() (string, error) {
	// Allow explicit override for deploys/containers.
	if p := os.Getenv("OFC_USER_SERVICE_CONFIG_PATH"); p != "" {
		return p, nil
	}

	// Support running from the repo root or from the service directory.
	candidates := []string{
		filepath.Join(".", "conf", configFileName),
		filepath.Join("..", "conf", configFileName),
		filepath.Join("user-service", "conf", configFileName),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", errors.New("config.json not found; set OFC_USER_SERVICE_CONFIG_PATH or place conf/config.json in expected locations")
}
