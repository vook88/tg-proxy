package main

import (
	"crypto/rand"
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

// GenerateSecret creates a 16-byte random secret for mtprotoproxy.
// Returns the 32 hex char secret (stored in config.py and DB).
func (p *ProxyManager) GenerateSecret() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(random), nil
}

// ProxyLink returns an https://t.me/proxy link for the given hex secret.
// For fake-TLS the link secret is: ee + raw_secret + hex(domain).
func (p *ProxyManager) ProxyLink(hexSecret string) string {
	domainHex := hex.EncodeToString([]byte(p.cfg.FakeTLSHost))
	linkSecret := "ee" + hexSecret + domainHex
	return fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=%s",
		p.cfg.ServerHost, p.cfg.ServerPort, linkSecret)
}

// SyncConfig writes all active secrets to mtprotoproxy config.py and reloads the proxy.
func (p *ProxyManager) SyncConfig() error {
	secrets, err := p.db.GetAllActiveSecrets()
	if err != nil {
		return fmt.Errorf("get active secrets: %w", err)
	}

	var users []string
	for _, s := range secrets {
		label := fmt.Sprintf("u%d", s.ID)
		users = append(users, fmt.Sprintf("    %q: %q,", label, s.HexSecret))
	}

	config := fmt.Sprintf(`PORT = %d

USERS = {
%s
}

MODES = {
    "classic": False,
    "secure": False,
    "tls": True,
}

TLS_DOMAIN = %q
`, p.cfg.ServerPort, strings.Join(users, "\n"), p.cfg.FakeTLSHost)

	if err := os.WriteFile(p.cfg.ConfigFile, []byte(config), 0640); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	slog.Info("proxy config updated", "secrets", len(secrets))

	if len(secrets) == 0 {
		slog.Warn("no active secrets, stopping proxy")
		_ = exec.Command("sh", "-c", "systemctl stop mtprotoproxy").Run()
		return nil
	}

	// Send SIGUSR2 to reload config without dropping connections.
	if err := exec.Command("sh", "-c", p.cfg.ReloadCmd).Run(); err != nil {
		// If reload fails (proxy not running), try restart.
		slog.Warn("reload failed, restarting", "err", err)
		if err := exec.Command("sh", "-c", "systemctl restart mtprotoproxy").Run(); err != nil {
			return fmt.Errorf("restart proxy: %w", err)
		}
	}

	slog.Info("proxy reloaded")
	return nil
}
