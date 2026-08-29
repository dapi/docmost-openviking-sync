package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var configEnvironment = []string{
	"SYNC_CONFIG", "SYNC_MODE", "DOCMOST_API_URL", "DOCMOST_URL",
	"DOCMOST_API_TOKEN", "DOCMOST_TOKEN", "DOCMOST_EMAIL", "DOCMOST_PASSWORD",
	"OPENVIKING_URL", "OPENVIKING_API_KEY", "OPENVIKING_ROOT", "SYNC_STATE_PATH",
	"SYNC_INTERVAL", "DOCMOST_SPACE_ALLOWLIST", "DOCMOST_SPACE_DENYLIST",
	"WEBHOOK_LISTEN", "WEBHOOK_PATH", "DOCMOST_WEBHOOK_SECRET", "WEBHOOK_DEBOUNCE",
}

func TestLoadConfigUsesLocalOpenVikingDefaultsWithoutAFile(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("DOCMOST_API_URL", "https://docmost.example.test/api")
	t.Setenv("DOCMOST_API_TOKEN", "docmost-token")

	cfg, err := loadConfig(cliOptions{visited: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "sync" || cfg.OpenViking.URL != "http://127.0.0.1:1933" || cfg.OpenViking.Root != "viking://user/resources/docmost" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.StatePath != filepath.Join("data", "state.json") || cfg.Interval != "24h" {
		t.Fatalf("unexpected reconciliation defaults: %#v", cfg)
	}
	if cfg.Webhook.Listen != ":8080" || cfg.Webhook.Path != "/events/docmost" || cfg.Webhook.Debounce != "10s" {
		t.Fatalf("unexpected webhook defaults: %#v", cfg.Webhook)
	}
}

func TestConfigurationPrecedenceCLIThenEnvironmentThenFile(t *testing.T) {
	clearConfigEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.json")
	fileConfig := `{
  "mode": "sync",
  "docmost": {"url":"https://file-docmost.test/api","token":"file-token","email":"file-email","password":"file-password"},
  "openviking": {"url":"http://file-openviking:1933","api_key":"file-key","root":"viking://user/resources/file"},
  "state_path":"file-state.json","interval":"2h","allowlist":["file-allow"],"denylist":["file-deny"],
  "webhook":{"listen":":8000","path":"/file","secret":"file-secret","debounce":"2s"}
}`
	if err := os.WriteFile(path, []byte(fileConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	environment := map[string]string{
		"SYNC_CONFIG": path, "SYNC_MODE": "daemon",
		"DOCMOST_API_URL": "https://env-docmost.test/api", "DOCMOST_API_TOKEN": "env-token",
		"DOCMOST_EMAIL": "env-email", "DOCMOST_PASSWORD": "env-password",
		"OPENVIKING_URL": "http://env-openviking:1933", "OPENVIKING_API_KEY": "env-key",
		"OPENVIKING_ROOT": "viking://user/resources/env", "SYNC_STATE_PATH": "env-state.json",
		"SYNC_INTERVAL": "3h", "DOCMOST_SPACE_ALLOWLIST": "env-a, env-b",
		"DOCMOST_SPACE_DENYLIST": "env-c", "WEBHOOK_LISTEN": ":8001",
		"WEBHOOK_PATH": "/env", "DOCMOST_WEBHOOK_SECRET": "env-secret", "WEBHOOK_DEBOUNCE": "3s",
	}
	for key, value := range environment {
		t.Setenv(key, value)
	}

	envConfig, err := loadConfig(cliOptions{visited: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigValues(t, envConfig, "daemon", "https://env-docmost.test/api", "env-token", "env-email", "env-password", "http://env-openviking:1933", "env-key", "viking://user/resources/env", "env-state.json", "3h", []string{"env-a", "env-b"}, []string{"env-c"}, ":8001", "/env", "env-secret", "3s")

	opts, err := parseCLI([]string{
		"--config", path, "--mode", "sync",
		"--docmost-url", "https://cli-docmost.test/api", "--docmost-token", "cli-token",
		"--docmost-email", "cli-email", "--docmost-password", "cli-password",
		"--openviking-url", "http://cli-openviking:1933", "--openviking-api-key", "cli-key",
		"--openviking-root", "viking://user/resources/cli", "--state-path", "cli-state.json",
		"--interval", "4h", "--allowlist", "cli-a,cli-b", "--denylist", "cli-c",
		"--webhook-listen", ":8002", "--webhook-path", "/cli",
		"--webhook-secret", "cli-secret", "--webhook-debounce", "4s",
	})
	if err != nil {
		t.Fatal(err)
	}
	cliConfig, err := loadConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	assertConfigValues(t, cliConfig, "sync", "https://cli-docmost.test/api", "cli-token", "cli-email", "cli-password", "http://cli-openviking:1933", "cli-key", "viking://user/resources/cli", "cli-state.json", "4h", []string{"cli-a", "cli-b"}, []string{"cli-c"}, ":8002", "/cli", "cli-secret", "4s")
}

func TestPositionalModeOverridesOtherLayers(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("SYNC_MODE", "sync")
	t.Setenv("DOCMOST_API_URL", "https://docmost.example.test/api")
	t.Setenv("DOCMOST_API_TOKEN", "token")
	opts, err := parseCLI([]string{"daemon"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "daemon" {
		t.Fatalf("mode = %q, want daemon", cfg.Mode)
	}
}

func assertConfigValues(t *testing.T, cfg config, mode, docmostURL, docmostToken, docmostEmail, docmostPassword, openVikingURL, openVikingAPIKey, openVikingRoot, statePath, interval string, allowlist, denylist []string, webhookListen, webhookPath, webhookSecret, webhookDebounce string) {
	t.Helper()
	got := []any{cfg.Mode, cfg.Docmost.URL, cfg.Docmost.Token, cfg.Docmost.Email, cfg.Docmost.Password, cfg.OpenViking.URL, cfg.OpenViking.APIKey, cfg.OpenViking.Root, cfg.StatePath, cfg.Interval, cfg.Allowlist, cfg.Denylist, cfg.Webhook.Listen, cfg.Webhook.Path, cfg.Webhook.Secret, cfg.Webhook.Debounce}
	want := []any{mode, docmostURL, docmostToken, docmostEmail, docmostPassword, openVikingURL, openVikingAPIKey, openVikingRoot, statePath, interval, allowlist, denylist, webhookListen, webhookPath, webhookSecret, webhookDebounce}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config values = %#v, want %#v", got, want)
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range configEnvironment {
		value, exists := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		key, value, exists := key, value, exists
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}
