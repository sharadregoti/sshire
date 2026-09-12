// Package config loads sshire's YAML configuration: company branding, the
// job list, and which sinks a submitted application should fan out to.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/sharadregoti/sshire/internal/model"
)

type Config struct {
	Company Company     `yaml:"company"`
	SSH     SSH         `yaml:"ssh"`
	Jobs    []model.Job `yaml:"jobs"`
	Sinks   Sinks       `yaml:"sinks"`
}

type Company struct {
	Name    string `yaml:"name"`
	Tagline string `yaml:"tagline"`

	// ApplyEmail is the fallback "just email us" address for any job that
	// doesn't set its own apply_email. Leave empty if every job uses the
	// in-TUI apply form instead.
	ApplyEmail string `yaml:"apply_email"`
}

type SSH struct {
	ListenAddr  string `yaml:"listen_addr"`
	HostKeyPath string `yaml:"host_key_path"`
}

type Sinks struct {
	Webhook   *WebhookSink   `yaml:"webhook"`
	Slack     *WebhookSink   `yaml:"slack"`
	ATS       *ATSSink       `yaml:"ats"`
	Dashboard *DashboardSink `yaml:"dashboard"`
}

type WebhookSink struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// ATSSink pushes applications into an existing applicant tracking system.
// Provider-specific field requirements vary; see internal/sink for the
// implementation notes and what has and hasn't been verified against a
// live account.
type ATSSink struct {
	Enabled    bool   `yaml:"enabled"`
	Provider   string `yaml:"provider"` // "greenhouse"
	BoardToken string `yaml:"board_token"`
	APIKey     string `yaml:"api_key"`
}

type DashboardSink struct {
	Enabled       bool   `yaml:"enabled"`
	DBPath        string `yaml:"db_path"`
	ListenAddr    string `yaml:"listen_addr"`
	AdminUser     string `yaml:"admin_user"`
	AdminPassword string `yaml:"admin_password"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if cfg.SSH.ListenAddr == "" {
		cfg.SSH.ListenAddr = ":2222"
	}
	if cfg.SSH.HostKeyPath == "" {
		cfg.SSH.HostKeyPath = ".sshire/host_key"
	}
	if len(cfg.Jobs) == 0 {
		return nil, fmt.Errorf("config %s: no jobs defined", path)
	}
	for i, j := range cfg.Jobs {
		if j.ID == "" || j.Title == "" {
			return nil, fmt.Errorf("config %s: jobs[%d] needs at least id and title", path, i)
		}
	}
	return &cfg, nil
}
