package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dapi/docmost-openviking-sync/internal/docmost"
	"github.com/dapi/docmost-openviking-sync/internal/openviking"
	"github.com/dapi/docmost-openviking-sync/internal/syncer"
	webhookserver "github.com/dapi/docmost-openviking-sync/internal/webhook"
)

var version = "dev"

type config struct {
	Mode    string `json:"mode"`
	Docmost struct {
		URL      string `json:"url"`
		Token    string `json:"token"`
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"docmost"`
	OpenViking struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
		Root   string `json:"root"`
	} `json:"openviking"`
	StatePath string   `json:"state_path"`
	Interval  string   `json:"interval"`
	Allowlist []string `json:"allowlist"`
	Denylist  []string `json:"denylist"`
	Webhook   struct {
		Listen   string `json:"listen"`
		Path     string `json:"path"`
		Secret   string `json:"secret"`
		Debounce string `json:"debounce"`
	} `json:"webhook"`
}

type cliOptions struct {
	ConfigPath       string
	Mode             string
	DocmostURL       string
	DocmostToken     string
	DocmostEmail     string
	DocmostPassword  string
	OpenVikingURL    string
	OpenVikingAPIKey string
	OpenVikingRoot   string
	StatePath        string
	Interval         string
	Allowlist        string
	Denylist         string
	WebhookListen    string
	WebhookPath      string
	WebhookSecret    string
	WebhookDebounce  string
	ShowVersion      bool
	visited          map[string]bool
}

func main() {
	opts, err := parseCLI(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	if opts.ShowVersion {
		fmt.Println(version)
		return
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.Mode == "sync" {
		if run(ctx, cfg) {
			os.Exit(1)
		}
		return
	}
	if err := runDaemon(ctx, cfg); err != nil {
		slog.Error("daemon stopped", "error", err)
		os.Exit(1)
	}
}

func parseCLI(args []string) (cliOptions, error) {
	var opts cliOptions
	flags := flag.NewFlagSet("docmost-openviking-sync", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&opts.ConfigPath, "config", "", "path to optional JSON configuration")
	flags.StringVar(&opts.Mode, "mode", "", "run mode: sync or daemon")
	flags.StringVar(&opts.DocmostURL, "docmost-url", "", "Docmost API URL")
	flags.StringVar(&opts.DocmostToken, "docmost-token", "", "Docmost API token")
	flags.StringVar(&opts.DocmostEmail, "docmost-email", "", "Docmost login email")
	flags.StringVar(&opts.DocmostPassword, "docmost-password", "", "Docmost login password")
	flags.StringVar(&opts.OpenVikingURL, "openviking-url", "", "OpenViking API URL")
	flags.StringVar(&opts.OpenVikingAPIKey, "openviking-api-key", "", "OpenViking API key")
	flags.StringVar(&opts.OpenVikingRoot, "openviking-root", "", "OpenViking destination root")
	flags.StringVar(&opts.StatePath, "state-path", "", "path to persistent sync state")
	flags.StringVar(&opts.Interval, "interval", "", "full reconciliation interval")
	flags.StringVar(&opts.Allowlist, "allowlist", "", "comma-separated Docmost space IDs or slugs to include")
	flags.StringVar(&opts.Denylist, "denylist", "", "comma-separated Docmost space IDs or slugs to exclude")
	flags.StringVar(&opts.WebhookListen, "webhook-listen", "", "webhook HTTP listen address")
	flags.StringVar(&opts.WebhookPath, "webhook-path", "", "webhook HTTP path")
	flags.StringVar(&opts.WebhookSecret, "webhook-secret", "", "Docmost webhook HMAC secret")
	flags.StringVar(&opts.WebhookDebounce, "webhook-debounce", "", "webhook reconciliation debounce")
	flags.BoolVar(&opts.ShowVersion, "version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	opts.visited = make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { opts.visited[f.Name] = true })
	if flags.NArg() > 1 {
		return opts, fmt.Errorf("usage: docmost-openviking-sync [flags] [sync|daemon]")
	}
	if flags.NArg() == 1 {
		opts.Mode = flags.Arg(0)
		opts.visited["mode"] = true
	}
	return opts, nil
}

func runDaemon(ctx context.Context, cfg config) error {
	interval, _ := time.ParseDuration(cfg.Interval)
	debounce, _ := time.ParseDuration(cfg.Webhook.Debounce)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	trigger := make(chan struct{}, 1)
	serverErrors := make(chan error, 1)
	var server *http.Server
	if cfg.Webhook.Secret != "" {
		mux := http.NewServeMux()
		mux.Handle(cfg.Webhook.Path, webhookserver.NewHandler(cfg.Webhook.Secret, func() {
			select {
			case trigger <- struct{}{}:
			default:
			}
		}))
		server = &http.Server{Addr: cfg.Webhook.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErrors <- err
			}
		}()
		defer server.Shutdown(context.Background())
		slog.Info("webhook receiver listening", "address", cfg.Webhook.Listen, "path", cfg.Webhook.Path)
	}

	if run(ctx, cfg) {
		slog.Error("initial sync failed")
	}

	var debounceTimer *time.Timer
	var debounceC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverErrors:
			return fmt.Errorf("webhook server: %w", err)
		case <-ticker.C:
			run(ctx, cfg)
		case <-trigger:
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(debounce)
				debounceC = debounceTimer.C
			} else {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(debounce)
			}
		case <-debounceC:
			debounceC = nil
			run(ctx, cfg)
		}
	}
}
func run(ctx context.Context, cfg config) bool {
	state, err := syncer.LoadState(cfg.StatePath)
	if err != nil {
		slog.Error("state error", "error", err)
		return true
	}
	r := syncer.Reconciler{Source: &docmost.Client{BaseURL: cfg.Docmost.URL, Token: cfg.Docmost.Token, Email: cfg.Docmost.Email, Password: cfg.Docmost.Password}, Sink: openviking.Client{BaseURL: cfg.OpenViking.URL, APIKey: cfg.OpenViking.APIKey}, Root: cfg.OpenViking.Root, DocmostURL: cfg.Docmost.URL, Allowlist: cfg.Allowlist, Denylist: cfg.Denylist, State: state}
	report := r.Sync(ctx)
	if err := syncer.SaveState(cfg.StatePath, r.State); err != nil {
		report.Errors = append(report.Errors, syncer.ItemError{Stage: "save_state", Message: err.Error()})
	}
	json.NewEncoder(os.Stdout).Encode(report)
	return report.Failed()
}
func loadConfig(opts cliOptions) (config, error) {
	c := defaultConfig()
	path := opts.ConfigPath
	if !opts.visited["config"] {
		if value, ok := os.LookupEnv("SYNC_CONFIG"); ok {
			path = value
		}
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return c, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := json.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("parse config %q: %w", path, err)
		}
	}

	applyEnvironment(&c)
	applyCLI(&c, opts)

	if c.Mode != "sync" && c.Mode != "daemon" {
		return c, fmt.Errorf("mode must be sync or daemon")
	}
	interval, err := time.ParseDuration(c.Interval)
	if err != nil || interval < time.Second {
		return c, fmt.Errorf("interval must be a Go duration of at least 1s")
	}
	debounce, err := time.ParseDuration(c.Webhook.Debounce)
	if err != nil || debounce < 100*time.Millisecond {
		return c, fmt.Errorf("webhook.debounce must be a Go duration of at least 100ms")
	}
	if !strings.HasPrefix(c.Webhook.Path, "/") {
		return c, fmt.Errorf("webhook.path must start with /")
	}
	if c.Webhook.Secret != "" && c.Webhook.Listen == "" {
		return c, fmt.Errorf("webhook.listen is required when webhook.secret is set")
	}
	if c.Docmost.URL == "" || (c.Docmost.Token == "" && (c.Docmost.Email == "" || c.Docmost.Password == "")) || c.OpenViking.URL == "" {
		return c, fmt.Errorf("docmost.url, Docmost credentials (token or email/password), and openviking.url are required")
	}
	return c, nil
}

func defaultConfig() config {
	var c config
	c.Mode = "sync"
	c.OpenViking.URL = "http://127.0.0.1:1933"
	c.OpenViking.Root = "viking://user/resources/docmost"
	c.StatePath = filepath.Join("data", "state.json")
	c.Interval = "24h"
	c.Webhook.Listen = ":8080"
	c.Webhook.Path = "/events/docmost"
	c.Webhook.Debounce = "10s"
	return c
}

func applyEnvironment(c *config) {
	setFromEnv(&c.Mode, "SYNC_MODE")
	setFromEnv(&c.Docmost.URL, "DOCMOST_API_URL", "DOCMOST_URL")
	setFromEnv(&c.Docmost.Token, "DOCMOST_API_TOKEN", "DOCMOST_TOKEN")
	setFromEnv(&c.Docmost.Email, "DOCMOST_EMAIL")
	setFromEnv(&c.Docmost.Password, "DOCMOST_PASSWORD")
	setFromEnv(&c.OpenViking.URL, "OPENVIKING_URL")
	setFromEnv(&c.OpenViking.APIKey, "OPENVIKING_API_KEY")
	setFromEnv(&c.OpenViking.Root, "OPENVIKING_ROOT")
	setFromEnv(&c.StatePath, "SYNC_STATE_PATH")
	setFromEnv(&c.Interval, "SYNC_INTERVAL")
	setCSVFromEnv(&c.Allowlist, "DOCMOST_SPACE_ALLOWLIST")
	setCSVFromEnv(&c.Denylist, "DOCMOST_SPACE_DENYLIST")
	setFromEnv(&c.Webhook.Listen, "WEBHOOK_LISTEN")
	setFromEnv(&c.Webhook.Path, "WEBHOOK_PATH")
	setFromEnv(&c.Webhook.Secret, "DOCMOST_WEBHOOK_SECRET")
	setFromEnv(&c.Webhook.Debounce, "WEBHOOK_DEBOUNCE")
}

func applyCLI(c *config, opts cliOptions) {
	setCLI(&c.Mode, opts.Mode, opts, "mode")
	setCLI(&c.Docmost.URL, opts.DocmostURL, opts, "docmost-url")
	setCLI(&c.Docmost.Token, opts.DocmostToken, opts, "docmost-token")
	setCLI(&c.Docmost.Email, opts.DocmostEmail, opts, "docmost-email")
	setCLI(&c.Docmost.Password, opts.DocmostPassword, opts, "docmost-password")
	setCLI(&c.OpenViking.URL, opts.OpenVikingURL, opts, "openviking-url")
	setCLI(&c.OpenViking.APIKey, opts.OpenVikingAPIKey, opts, "openviking-api-key")
	setCLI(&c.OpenViking.Root, opts.OpenVikingRoot, opts, "openviking-root")
	setCLI(&c.StatePath, opts.StatePath, opts, "state-path")
	setCLI(&c.Interval, opts.Interval, opts, "interval")
	if opts.visited["allowlist"] {
		c.Allowlist = parseCSV(opts.Allowlist)
	}
	if opts.visited["denylist"] {
		c.Denylist = parseCSV(opts.Denylist)
	}
	setCLI(&c.Webhook.Listen, opts.WebhookListen, opts, "webhook-listen")
	setCLI(&c.Webhook.Path, opts.WebhookPath, opts, "webhook-path")
	setCLI(&c.Webhook.Secret, opts.WebhookSecret, opts, "webhook-secret")
	setCLI(&c.Webhook.Debounce, opts.WebhookDebounce, opts, "webhook-debounce")
}

func setFromEnv(target *string, keys ...string) {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			*target = value
			return
		}
	}
}

func setCSVFromEnv(target *[]string, key string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = parseCSV(value)
	}
}

func setCLI(target *string, value string, opts cliOptions, name string) {
	if opts.visited[name] {
		*target = value
	}
}

func parseCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
