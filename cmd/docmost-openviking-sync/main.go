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

func main() {
	var path string
	var showVersion bool
	flag.StringVar(&path, "config", env("SYNC_CONFIG", "config.json"), "path to JSON configuration")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(version)
		return
	}
	mode := "sync"
	if flag.NArg() > 0 {
		mode = flag.Arg(0)
	}
	if mode != "sync" && mode != "daemon" {
		fmt.Fprintln(os.Stderr, "usage: docmost-openviking-sync [-config file] [sync|daemon]")
		os.Exit(2)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if mode == "sync" {
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
func loadConfig(path string) (config, error) {
	var c config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	c.Docmost.URL = first(os.Getenv("DOCMOST_API_URL"), c.Docmost.URL)
	c.Docmost.Token = first(os.Getenv("DOCMOST_API_TOKEN"), os.Getenv("DOCMOST_TOKEN"), c.Docmost.Token)
	c.Docmost.Email = first(os.Getenv("DOCMOST_EMAIL"), c.Docmost.Email)
	c.Docmost.Password = first(os.Getenv("DOCMOST_PASSWORD"), c.Docmost.Password)
	c.OpenViking.URL = first(os.Getenv("OPENVIKING_URL"), c.OpenViking.URL)
	c.OpenViking.APIKey = first(os.Getenv("OPENVIKING_API_KEY"), c.OpenViking.APIKey)
	c.Webhook.Listen = first(os.Getenv("WEBHOOK_LISTEN"), c.Webhook.Listen)
	c.Webhook.Path = first(os.Getenv("WEBHOOK_PATH"), c.Webhook.Path)
	c.Webhook.Secret = first(os.Getenv("DOCMOST_WEBHOOK_SECRET"), c.Webhook.Secret)
	c.Webhook.Debounce = first(os.Getenv("WEBHOOK_DEBOUNCE"), c.Webhook.Debounce)
	if c.OpenViking.Root == "" {
		c.OpenViking.Root = "viking://user/resources/docmost"
	}
	if c.StatePath == "" {
		c.StatePath = filepath.Join("data", "state.json")
	}
	if c.Interval == "" {
		c.Interval = "24h"
	}
	if c.Webhook.Listen == "" {
		c.Webhook.Listen = ":8080"
	}
	if c.Webhook.Path == "" {
		c.Webhook.Path = "/events/docmost"
	}
	if c.Webhook.Debounce == "" {
		c.Webhook.Debounce = "10s"
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
	if c.Docmost.URL == "" || (c.Docmost.Token == "" && (c.Docmost.Email == "" || c.Docmost.Password == "")) || c.OpenViking.URL == "" {
		return c, fmt.Errorf("docmost.url, Docmost credentials (token or email/password), and openviking.url are required")
	}
	return c, nil
}
func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
