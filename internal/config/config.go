package config

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/ysqss/watchdog/internal/target"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Database DatabaseConfig  `yaml:"database"`
	Notifier NotifierConfig  `yaml:"notifier"`
	Watchdog WatchdogConfig  `yaml:"watchdog"`
	Targets  []target.Target `yaml:"targets"`
}

type ServerConfig struct {
	Listen            string `yaml:"listen"`
	AdminToken        string `yaml:"admin_token"`
	StatusPageEnabled bool   `yaml:"status_page_enabled"`
	DebugPort         string `yaml:"debug_port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type NotifierConfig struct {
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token"`
}

type WatchdogConfig struct {
	HeartbeatInterval    int `yaml:"heartbeat_interval"`
	CleanupInterval      int `yaml:"cleanup_interval"`
	ProbeHistoryDays     int `yaml:"probe_history_days"`
	EventHistoryDays     int `yaml:"event_history_days"`
	TLSExpiryWarningDays int `yaml:"tls_expiry_warning_days"`
}

func (w WatchdogConfig) HeartbeatDuration() time.Duration {
	if w.HeartbeatInterval <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(w.HeartbeatInterval) * time.Second
}

func (w WatchdogConfig) CleanupDuration() time.Duration {
	if w.CleanupInterval <= 0 {
		return time.Hour
	}
	return time.Duration(w.CleanupInterval) * time.Second
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:            ":8080",
			AdminToken:        "changeme",
			StatusPageEnabled: true,
		},
		Database: DatabaseConfig{
			Path: "data/watchdog.db",
		},
		Notifier: NotifierConfig{},
		Watchdog: WatchdogConfig{
			HeartbeatInterval:    300,
			CleanupInterval:      3600,
			ProbeHistoryDays:     30,
			EventHistoryDays:     90,
			TLSExpiryWarningDays: 30,
		},
		Targets: []target.Target{},
	}
}

type Manager struct {
	current atomic.Value
	path    string
}

func Load(path string) (*Manager, error) {
	m := &Manager{path: path}
	cfg := Default()
	if path != "" {
		if err := m.loadFile(cfg); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}
	m.current.Store(cfg)
	return m, nil
}

func (m *Manager) loadFile(cfg *Config) error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

func (m *Manager) Get() *Config {
	cfg, ok := m.current.Load().(*Config)
	if !ok {
		return Default()
	}
	return cfg
}

func (m *Manager) Reload() error {
	cfg := Default()
	if err := m.loadFile(cfg); err != nil {
		return err
	}
	m.current.Store(cfg)
	return nil
}
