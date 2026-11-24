package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ofc-user-service/logger"
)

var configFileName = "config.json"

// Stages. Anything other than development is treated as a real deployment:
// secrets must come from the environment and development shortcuts (OTP in
// the response, the auto-created dev tenant) are off.
const (
	StageDevelopment = "development"
	StageStaging     = "staging"
	StageProduction  = "production"
)

// minSecretLength is the minimum length of the OTP hash secret outside
// development (32 bytes of entropy when generated as hex/base64).
const minSecretLength = 32

type Config struct {
	Logger         logger.LoggerConfig        `json:"logger"`
	PostDataSource PostgreDataSourceConfig    `json:"postgreDataSource"`
	SeedData       SeedConfig                 `json:"seed"`
	Redis          RedisConfig                `json:"redis"`
	HTTP           HTTPConfig                 `json:"http"`
	OTP            OTPConfig                  `json:"otp"`
	Request        RequestConfig              `json:"request"`
	RateLimits     map[string]RateLimitConfig `json:"rateLimits"`
	DevTenant      DevTenantConfig            `json:"devTenant"`
	// Stage is development, staging or production. It defaults to
	// production, so a missing setting can never enable development mode.
	Stage string `json:"stage"`

	// fromEnv records which secrets were supplied by environment variables;
	// outside development they must be (see validate).
	fromEnv map[string]bool
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
	// TrustedProxies lists the IPs / CIDRs of load balancers in front of the
	// service. Only requests arriving from one of them have their
	// X-Forwarded-For header honoured when resolving the client IP.
	TrustedProxies []string `json:"trustedProxies"`
}

// TrustedProxyPrefixes parses TrustedProxies (validated at load time).
func (h HTTPConfig) TrustedProxyPrefixes() []netip.Prefix {
	prefixes, _ := parsePrefixes(h.TrustedProxies)
	return prefixes
}

type OTPConfig struct {
	Length                int    `json:"length"`
	TTLSeconds            int    `json:"ttlSeconds"`
	ResendCooldownSeconds int    `json:"resendCooldownSeconds"`
	MaxRequestsPerHour    int    `json:"maxRequestsPerHour"`
	MaxVerifyAttempts     int    `json:"maxVerifyAttempts"`
	HashSecret            string `json:"hashSecret"`
}

// RateLimitConfig overrides one named route rate limit (see
// service.DefaultRateLimits for the names and defaults).
type RateLimitConfig struct {
	Limit         int `json:"limit"`
	WindowSeconds int `json:"windowSeconds"`
}

type DevTenantConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RequestConfig struct {
	ReplayWindowSeconds int `json:"replayWindowSeconds"`
}

// SeedConfig points at the directory containing the seed JSON files.
// Every file in this directory is matched by name (without extension) to a
// registered table/model in service.SeedRegistry.
type SeedConfig struct {
	Dir string `json:"dir"`
}

// PostgreDataSourceConfig holds two sets of credentials:
//
//   - User / Password: the role the service runs as. It must NOT be a
//     superuser or have BYPASSRLS, otherwise row-level security (tenant
//     isolation) is silently skipped. The service refuses to start outside
//     development if it is.
//   - AdminUser / AdminPassword: only used when EnableAutoMigrate is on, to
//     create the database, run migrations and create/grant the app role.
type PostgreDataSourceConfig struct {
	DriverName        string `json:"driverName"`
	Addr              string `json:"addr"`
	Port              string `json:"port"`
	Database          string `json:"database"`
	User              string `json:"user"`
	Password          string `json:"password"`
	AdminUser         string `json:"adminUser"`
	AdminPassword     string `json:"adminPassword"`
	SSLMode           string `json:"sslMode"`
	EnableAutoMigrate bool   `json:"enableAutoMigrate"`

	MaxOpenConns           int `json:"maxOpenConns"`
	MaxIdleConns           int `json:"maxIdleConns"`
	ConnMaxLifetimeMinutes int `json:"connMaxLifetimeMinutes"`
	ConnMaxIdleMinutes     int `json:"connMaxIdleMinutes"`
	// StatementTimeoutMs caps every query the app runs; a stuck query fails
	// instead of holding a pooled connection forever.
	StatementTimeoutMs int `json:"statementTimeoutMs"`
	// ConnectAttempts bounds startup retries while the database comes up.
	ConnectAttempts int `json:"connectAttempts"`
}

var config Config
var configOnce sync.Once

// GetConfig loads conf/config.json once, applies environment overrides and
// defaults, and validates the result. It panics on an invalid config: the
// service must not start half-configured.
func GetConfig() *Config {
	configOnce.Do(func() {
		path, err := resolveConfigPath()
		if err != nil {
			panic(err)
		}
		loaded, err := Load(path, os.Getenv)
		if err != nil {
			panic(err)
		}
		config = *loaded
	})
	return &config
}

// Load reads the config file at path, applies environment overrides (read
// through getenv), fills defaults and validates it.
func Load(path string, getenv func(string) string) (*Config, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(bytes, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := applyEnv(&c, getenv); err != nil {
		return nil, err
	}
	applyDefaults(&c)
	if err := validate(&c); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &c, nil
}

// applyEnv lets deployments override settings without editing config.json.
// Secrets in particular are expected to come from here outside development.
func applyEnv(c *Config, getenv func(string) string) error {
	c.fromEnv = map[string]bool{}
	str := func(name string, dst *string) {
		if v := getenv(name); v != "" {
			*dst = v
			c.fromEnv[name] = true
		}
	}
	var errs []error
	integer := func(name string, dst *int) {
		if v := getenv(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s must be an integer", name))
				return
			}
			*dst = n
		}
	}

	str("OFC_STAGE", &c.Stage)
	str("OFC_HTTP_ADDR", &c.HTTP.Addr)
	if v := getenv("OFC_HTTP_TRUSTED_PROXIES"); v != "" {
		c.HTTP.TrustedProxies = splitList(v)
	}

	db := &c.PostDataSource
	str("OFC_DB_HOST", &db.Addr)
	str("OFC_DB_PORT", &db.Port)
	str("OFC_DB_NAME", &db.Database)
	str("OFC_DB_USER", &db.User)
	str("OFC_DB_PASSWORD", &db.Password)
	str("OFC_DB_ADMIN_USER", &db.AdminUser)
	str("OFC_DB_ADMIN_PASSWORD", &db.AdminPassword)
	str("OFC_DB_SSLMODE", &db.SSLMode)
	integer("OFC_DB_MAX_OPEN_CONNS", &db.MaxOpenConns)
	if v := getenv("OFC_DB_AUTO_MIGRATE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, errors.New("OFC_DB_AUTO_MIGRATE must be true or false"))
		}
		db.EnableAutoMigrate = b
	}

	str("OFC_REDIS_ADDR", &c.Redis.Addr)
	str("OFC_REDIS_PASSWORD", &c.Redis.Password)
	str("OFC_OTP_HASH_SECRET", &c.OTP.HashSecret)
	return errors.Join(errs...)
}

func applyDefaults(c *Config) {
	if c.Stage == "" {
		c.Stage = StageProduction
	}
	if c.PostDataSource.SSLMode == "" {
		c.PostDataSource.SSLMode = "disable"
	}
	db := &c.PostDataSource
	if db.MaxOpenConns <= 0 {
		db.MaxOpenConns = 50
	}
	if db.MaxIdleConns <= 0 || db.MaxIdleConns > db.MaxOpenConns {
		db.MaxIdleConns = db.MaxOpenConns / 2
	}
	if db.ConnMaxLifetimeMinutes <= 0 {
		db.ConnMaxLifetimeMinutes = 30
	}
	if db.ConnMaxIdleMinutes <= 0 {
		db.ConnMaxIdleMinutes = 5
	}
	if db.StatementTimeoutMs <= 0 {
		db.StatementTimeoutMs = 5000
	}
	if db.ConnectAttempts <= 0 {
		db.ConnectAttempts = 10
	}
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

func validate(c *Config) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	switch c.Stage {
	case StageDevelopment, StageStaging, StageProduction:
	default:
		fail("stage must be %q, %q or %q, got %q", StageDevelopment, StageStaging, StageProduction, c.Stage)
	}

	db := c.PostDataSource
	if db.Addr == "" || db.Port == "" || db.Database == "" || db.User == "" {
		fail("postgreDataSource addr, port, database and user are required")
	}
	if db.AdminUser != "" && db.AdminUser == db.User {
		fail("postgreDataSource.user must be a dedicated app role, not the admin user %q", db.AdminUser)
	}
	if db.EnableAutoMigrate && db.AdminUser == "" {
		fail("postgreDataSource.adminUser (or OFC_DB_ADMIN_USER) is required when enableAutoMigrate is on")
	}
	if _, err := parsePrefixes(c.HTTP.TrustedProxies); err != nil {
		fail("http.trustedProxies: %v", err)
	}
	for name, rl := range c.RateLimits {
		if rl.Limit <= 0 || rl.WindowSeconds <= 0 {
			fail("rateLimits.%s: limit and windowSeconds must be positive", name)
		}
	}

	if !c.IsDevelopment() {
		// Real deployments: secrets only from the environment, never from
		// the config file that ships in the repo / image.
		if !c.fromEnv["OFC_OTP_HASH_SECRET"] {
			fail("OFC_OTP_HASH_SECRET must be set in the environment outside development")
		} else if len(c.OTP.HashSecret) < minSecretLength {
			fail("OFC_OTP_HASH_SECRET must be at least %d characters", minSecretLength)
		}
		if !c.fromEnv["OFC_DB_PASSWORD"] {
			fail("OFC_DB_PASSWORD must be set in the environment outside development")
		}
		if db.EnableAutoMigrate && !c.fromEnv["OFC_DB_ADMIN_PASSWORD"] {
			fail("OFC_DB_ADMIN_PASSWORD must be set in the environment when auto-migrating outside development")
		}
	}
	return errors.Join(errs...)
}

// parsePrefixes accepts single IPs ("10.0.0.5") and CIDRs ("10.0.0.0/8").
func parsePrefixes(values []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if strings.Contains(v, "/") {
			p, err := netip.ParsePrefix(v)
			if err != nil {
				return nil, fmt.Errorf("%q is not a valid CIDR", v)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("%q is not a valid IP", v)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func splitList(v string) []string {
	var out []string
	for part := range strings.SplitSeq(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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
