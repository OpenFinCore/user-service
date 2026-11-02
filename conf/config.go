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

const StageDevelopment = "development"

type Config struct {
	Logger         logger.LoggerConfig     `json:"logger"`
	PostDataSource PostgreDataSourceConfig `json:"postgreDataSource"`
	SeedData       SeedConfig              `json:"seed"`
	Redis          RedisConfig             `json:"redis"`
	HTTP           HTTPConfig              `json:"http"`
	OTP            OTPConfig               `json:"otp"`
	Request        RequestConfig           `json:"request"`
	DevTenant      DevTenantConfig         `json:"devTenant"`
	Stage          string                  `json:"stage"`
}

func (c *Config) IsDevelopment() bool { return c.Stage == StageDevelopment }

type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type HTTPConfig struct {
	Addr                   string `json:"addr"`
	ReadTimeoutSeconds     int    `json:"readTimeoutSeconds"`
	WriteTimeoutSeconds    int    `json:"writeTimeoutSeconds"`
	ShutdownTimeoutSeconds int    `json:"shutdownTimeoutSeconds"`
}

type OTPConfig struct {
	Length                int    `json:"length"`
	TTLSeconds            int    `json:"ttlSeconds"`
	ResendCooldownSeconds int    `json:"resendCooldownSeconds"`
	MaxRequestsPerHour    int    `json:"maxRequestsPerHour"`
	MaxVerifyAttempts     int    `json:"maxVerifyAttempts"`
	HashSecret            string `json:"hashSecret"`
}

// SeedConfig points at the directory containing the seed JSON files.
// Every file in this directory is matched by name (without extension) to a
// registered table/model in service.SeedRegistry.
type DevTenantConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RequestConfig struct {
	ReplayWindowSeconds int `json:"replayWindowSeconds"`
}

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

		applyDefaults(&config)

		if pw := os.Getenv("OFC_REDIS_PASSWORD"); pw != "" {
			config.Redis.Password = pw
		}
		if secret := os.Getenv("OFC_OTP_HASH_SECRET"); secret != "" {
			config.OTP.HashSecret = secret
		}
		if config.OTP.HashSecret == "" && !config.IsDevelopment() {
			panic(errors.New("otp.hashSecret (or OFC_OTP_HASH_SECRET) is required outside development"))
		}
	})
	return &config
}

func applyDefaults(c *Config) {
	if c.Redis.Addr == "" {
		c.Redis.Addr = "127.0.0.1:6379"
	}
	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":8080"
	}
	if c.HTTP.ReadTimeoutSeconds <= 0 {
		c.HTTP.ReadTimeoutSeconds = 10
	}
	if c.HTTP.WriteTimeoutSeconds <= 0 {
		c.HTTP.WriteTimeoutSeconds = 10
	}
	if c.HTTP.ShutdownTimeoutSeconds <= 0 {
		c.HTTP.ShutdownTimeoutSeconds = 15
	}
	if c.DevTenant.ID == "" {
		c.DevTenant.ID = "d0d0d0d0-0000-4000-8000-000000000001"
	}
	if c.DevTenant.Name == "" {
		c.DevTenant.Name = "Development Tenant"
	}
	if c.Request.ReplayWindowSeconds <= 0 {
		c.Request.ReplayWindowSeconds = 300
	}
	if c.OTP.Length <= 0 {
		c.OTP.Length = 6
	}
	if c.OTP.TTLSeconds <= 0 {
		c.OTP.TTLSeconds = 300
	}
	if c.OTP.ResendCooldownSeconds <= 0 {
		c.OTP.ResendCooldownSeconds = 60
	}
	if c.OTP.MaxRequestsPerHour <= 0 {
		c.OTP.MaxRequestsPerHour = 5
	}
	if c.OTP.MaxVerifyAttempts <= 0 {
		c.OTP.MaxVerifyAttempts = 5
	}
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
