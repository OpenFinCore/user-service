package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseConfig = `{
  "postgreDataSource": {"addr": "127.0.0.1", "port": "5432", "database": "ofc", "user": "ofc_app", "password": "file-pw",
                        "adminUser": "postgres", "adminPassword": "file-admin-pw", "enableAutoMigrate": true},
  "otp": {"hashSecret": "dev-only-secret"},
  "stage": "%s"
}`

func load(t *testing.T, stage string, env map[string]string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(strings.Replace(baseConfig, "%s", stage, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path, func(k string) string { return env[k] })
}

var prodEnv = map[string]string{
	"OFC_OTP_HASH_SECRET":   strings.Repeat("s", 32),
	"OFC_DB_PASSWORD":       "env-pw",
	"OFC_DB_ADMIN_PASSWORD": "env-admin-pw",
}

func TestLoad_DevelopmentAcceptsFileSecrets(t *testing.T) {
	c, err := load(t, "development", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsDevelopment() || c.OTP.HashSecret != "dev-only-secret" {
		t.Fatalf("unexpected config %+v", c)
	}
}

func TestLoad_MissingStageDefaultsToProduction(t *testing.T) {
	// No stage and no env secrets: must NOT silently run in development.
	if _, err := load(t, "", nil); err == nil || !strings.Contains(err.Error(), "OFC_OTP_HASH_SECRET") {
		t.Fatalf("want production validation error, got %v", err)
	}
	c, err := load(t, "", prodEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.Stage != StageProduction || c.IsDevelopment() {
		t.Fatalf("want production, got %q", c.Stage)
	}
}

func TestLoad_ProductionRequiresEnvSecrets(t *testing.T) {
	for _, missing := range []string{"OFC_OTP_HASH_SECRET", "OFC_DB_PASSWORD", "OFC_DB_ADMIN_PASSWORD"} {
		env := map[string]string{}
		for k, v := range prodEnv {
			if k != missing {
				env[k] = v
			}
		}
		if _, err := load(t, "production", env); err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("missing %s: want error naming it, got %v", missing, err)
		}
	}
}

func TestLoad_ProductionRejectsShortSecret(t *testing.T) {
	env := map[string]string{"OFC_OTP_HASH_SECRET": "short", "OFC_DB_PASSWORD": "x", "OFC_DB_ADMIN_PASSWORD": "y"}
	if _, err := load(t, "production", env); err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Fatalf("want short-secret error, got %v", err)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	env := map[string]string{"OFC_STAGE": "development", "OFC_DB_HOST": "db.internal", "OFC_HTTP_TRUSTED_PROXIES": "10.0.0.0/8, 192.168.1.5"}
	c, err := load(t, "production", env)
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsDevelopment() || c.PostDataSource.Addr != "db.internal" || len(c.HTTP.TrustedProxyPrefixes()) != 2 {
		t.Fatalf("env overrides not applied: %+v", c)
	}
}

func TestLoad_RejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"stage":          {"OFC_STAGE": "prod"},
		"trustedProxies": {"OFC_STAGE": "development", "OFC_HTTP_TRUSTED_PROXIES": "not-an-ip"},
		"app role":       {"OFC_STAGE": "development", "OFC_DB_USER": "postgres"},
	}
	for want, env := range cases {
		if _, err := load(t, "development", env); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("want error mentioning %q, got %v", want, err)
		}
	}
}
