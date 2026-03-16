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

	// Path to the file where active secrets are written for mtg.
	SecretsFile string

	// Hostname used for fake-TLS SNI (e.g. "google.com").
	FakeTLSHost string

	// Command to restart mtg after secrets change.
	RestartCmd string
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

	secretsFile := os.Getenv("SECRETS_FILE")
	if secretsFile == "" {
		secretsFile = "/etc/mtg/secrets.txt"
	}

	fakeTLSHost := os.Getenv("FAKE_TLS_HOST")
	if fakeTLSHost == "" {
		fakeTLSHost = "google.com"
	}

	restartCmd := os.Getenv("RESTART_CMD")
	if restartCmd == "" {
		restartCmd = "systemctl restart mtg"
	}

	return &Config{
		BotToken:    token,
		AdminID:     adminID,
		DBPath:      dbPath,
		ServerHost:  serverHost,
		ServerPort:  port,
		SecretsFile: secretsFile,
		FakeTLSHost: fakeTLSHost,
		RestartCmd:  restartCmd,
	}, nil
}
