package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	BotToken    string
	AdminID     int64
	DBPath      string
	ServerHost  string
	ServerPort  int
	ConfigFile  string
	FakeTLSHost string
	ReloadCmd   string
	MetricsURL  string
}

func Load() (*Config, error) {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}

	adminIDStr := os.Getenv("ADMIN_ID")
	if adminIDStr == "" {
		return nil, fmt.Errorf("ADMIN_ID is required")
	}
	adminID, err := strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("ADMIN_ID must be a number: %w", err)
	}

	serverHost := os.Getenv("SERVER_HOST")
	if serverHost == "" {
		return nil, fmt.Errorf("SERVER_HOST is required")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/var/lib/tg-proxy/data.db"
	}

	portStr := os.Getenv("SERVER_PORT")
	if portStr == "" {
		portStr = "9443"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("SERVER_PORT must be a number: %w", err)
	}

	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "/etc/telemt/config.toml"
	}

	fakeTLSHost := os.Getenv("FAKE_TLS_HOST")
	if fakeTLSHost == "" {
		fakeTLSHost = "cloudflare.com"
	}

	reloadCmd := os.Getenv("RELOAD_CMD")
	if reloadCmd == "" {
		reloadCmd = "systemctl reload telemt"
	}

	metricsURL := os.Getenv("METRICS_URL")
	if metricsURL == "" {
		metricsURL = "http://127.0.0.1:8888/"
	}

	return &Config{
		BotToken:    token,
		AdminID:     adminID,
		DBPath:      dbPath,
		ServerHost:  serverHost,
		ServerPort:  port,
		ConfigFile:  configFile,
		FakeTLSHost: fakeTLSHost,
		ReloadCmd:   reloadCmd,
		MetricsURL:  metricsURL,
	}, nil
}
