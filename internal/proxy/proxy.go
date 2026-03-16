package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"tg-proxy/internal/config"
	"tg-proxy/internal/db"
)

type Manager struct {
	cfg *config.Config
	db  *db.DB
}

func NewManager(cfg *config.Config, db *db.DB) *Manager {
	return &Manager{cfg: cfg, db: db}
}

// GenerateSecret creates a 16-byte random secret for mtprotoproxy.
func (m *Manager) GenerateSecret() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(random), nil
}

// ProxyLink returns an https://t.me/proxy link for the given hex secret.
func (m *Manager) ProxyLink(hexSecret string) string {
	domainHex := hex.EncodeToString([]byte(m.cfg.FakeTLSHost))
	linkSecret := "ee" + hexSecret + domainHex
	return fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=%s",
		m.cfg.ServerHost, m.cfg.ServerPort, linkSecret)
}

// SyncConfig writes all active secrets to mtprotoproxy config.py and reloads the proxy.
func (m *Manager) SyncConfig() error {
	secrets, err := m.db.GetAllActiveSecrets()
	if err != nil {
		return fmt.Errorf("get active secrets: %w", err)
	}

	var users []string
	for _, s := range secrets {
		label := fmt.Sprintf("u%d", s.ID)
		users = append(users, fmt.Sprintf("    %q: %q,", label, s.HexSecret))
	}

	cfg := fmt.Sprintf(`PORT = %d

USERS = {
%s
}

MODES = {
    "classic": False,
    "secure": False,
    "tls": True,
}

TLS_DOMAIN = %q

METRICS_PORT = 8888
METRICS_WHITELIST = ["127.0.0.1"]
`, m.cfg.ServerPort, strings.Join(users, "\n"), m.cfg.FakeTLSHost)

	if err := os.WriteFile(m.cfg.ConfigFile, []byte(cfg), 0640); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	slog.Info("proxy config updated", "secrets", len(secrets))

	if len(secrets) == 0 {
		slog.Warn("no active secrets, stopping proxy")
		_ = exec.Command("sh", "-c", "systemctl stop mtprotoproxy").Run()
		return nil
	}

	// Send SIGUSR2 to reload config without dropping connections.
	if err := exec.Command("sh", "-c", m.cfg.ReloadCmd).Run(); err != nil {
		slog.Warn("reload failed, restarting", "err", err)
		if err := exec.Command("sh", "-c", "systemctl restart mtprotoproxy").Run(); err != nil {
			return fmt.Errorf("restart proxy: %w", err)
		}
	}

	slog.Info("proxy reloaded")
	return nil
}
