package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

type ProxyManager struct {
	cfg *Config
	db  *DB
}

func NewProxyManager(cfg *Config, db *DB) *ProxyManager {
	return &ProxyManager{cfg: cfg, db: db}
}

// GenerateSecret creates a new mtg-compatible fake-TLS secret.
// Returns hex-encoded (for Telegram links) and base64-encoded (for mtg config) forms.
func (p *ProxyManager) GenerateSecret() (hexSecret, b64Secret string, err error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}

	// Fake-TLS secret format: 0xee + 16 random bytes + hostname bytes
	full := make([]byte, 0, 1+16+len(p.cfg.FakeTLSHost))
	full = append(full, 0xee)
	full = append(full, random...)
	full = append(full, []byte(p.cfg.FakeTLSHost)...)

	hexSecret = hex.EncodeToString(full)
	b64Secret = base64.RawURLEncoding.EncodeToString(full)

	return hexSecret, b64Secret, nil
}

// ProxyLink returns a tg:// proxy link for the given hex secret.
func (p *ProxyManager) ProxyLink(hexSecret string) string {
	return fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=%s",
		p.cfg.ServerHost, p.cfg.ServerPort, hexSecret)
}

// SyncAndRestart writes all active secrets to the secrets file and restarts mtg.
func (p *ProxyManager) SyncAndRestart() error {
	secrets, err := p.db.GetAllActiveSecrets()
	if err != nil {
		return fmt.Errorf("get active secrets: %w", err)
	}

	var lines []string
	for _, s := range secrets {
		lines = append(lines, s.B64Secret)
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}

	if err := os.WriteFile(p.cfg.SecretsFile, []byte(content), 0640); err != nil {
		return fmt.Errorf("write secrets file: %w", err)
	}

	slog.Info("secrets file updated", "count", len(secrets))

	if len(secrets) == 0 {
		slog.Warn("no active secrets, stopping mtg")
		_ = exec.Command("sh", "-c", "systemctl stop mtg").Run()
		return nil
	}

	if err := exec.Command("sh", "-c", p.cfg.RestartCmd).Run(); err != nil {
		return fmt.Errorf("restart mtg: %w", err)
	}

	slog.Info("mtg restarted")
	return nil
}
