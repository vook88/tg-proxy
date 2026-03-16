package main

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// Telegram bot token from @BotFather.
	BotToken string

	// Telegram ID of the admin who approves requests.
	AdminID int64

	// Path to the SQLite database file.
	DBPath string

	// Public IP or domain of the proxy server.
	ServerHost string

	// Port on which mtg listens.
	ServerPort int

	// Path to mtprotoproxy config.py file.
	ConfigFile string

	// Hostname used for fake-TLS SNI (e.g. "google.com").
	FakeTLSHost string

	// Command to reload mtprotoproxy (SIGUSR2).
	ReloadCmd string

	// URL to fetch Prometheus metrics from mtprotoproxy.
	MetricsURL string
}

func LoadConfig() (*Config, error) {
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
		portStr = "443"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("SERVER_PORT must be a number: %w", err)
	}

	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "/opt/mtprotoproxy/config.py"
	}

	fakeTLSHost := os.Getenv("FAKE_TLS_HOST")
	if fakeTLSHost == "" {
		fakeTLSHost = "google.com"
	}

	reloadCmd := os.Getenv("RELOAD_CMD")
	if reloadCmd == "" {
		reloadCmd = "systemctl kill -s SIGUSR2 mtprotoproxy"
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
