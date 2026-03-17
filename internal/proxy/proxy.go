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

// SyncConfig writes all active secrets to telemt config.toml and reloads the proxy.
func (m *Manager) SyncConfig() error {
	secrets, err := m.db.GetAllActiveSecrets()
	if err != nil {
		return fmt.Errorf("get active secrets: %w", err)
	}

	var users []string
	for _, s := range secrets {
		label := fmt.Sprintf("u%d", s.ID)
		users = append(users, fmt.Sprintf("%s = %q", label, s.HexSecret))
	}

	cfg := fmt.Sprintf(`[general]
use_middle_proxy = true
log_level = "normal"

[general.modes]
classic = false
secure = false
tls = true

[general.links]
show = "*"
public_host = %q
public_port = %d

[server]
port = %d
metrics_port = 8888
metrics_listen = "127.0.0.1:8888"

[server.api]
enabled = true
listen = "127.0.0.1:9091"
whitelist = ["127.0.0.0/8"]

[[server.listeners]]
ip = "0.0.0.0"

[censorship]
tls_domain = %q
mask = true
tls_emulation = true
tls_front_dir = "/var/lib/telemt/tlsfront"

[access.users]
%s
`, m.cfg.ServerHost, m.cfg.ServerPort, m.cfg.ServerPort, m.cfg.FakeTLSHost, strings.Join(users, "\n"))

	if err := os.WriteFile(m.cfg.ConfigFile, []byte(cfg), 0640); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	slog.Info("proxy config updated", "secrets", len(secrets))

	if len(secrets) == 0 {
		slog.Warn("no active secrets, stopping proxy")
		_ = exec.Command("sh", "-c", "systemctl stop telemt").Run()
		return nil
	}

	if err := exec.Command("sh", "-c", m.cfg.ReloadCmd).Run(); err != nil {
		slog.Warn("reload failed, restarting", "err", err)
		if err := exec.Command("sh", "-c", "systemctl restart telemt").Run(); err != nil {
			return fmt.Errorf("restart proxy: %w", err)
		}
	}

	slog.Info("proxy reloaded")
	return nil
}
