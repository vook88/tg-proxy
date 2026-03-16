package main

import (
	"log/slog"
	"os"
	"path/filepath"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	// Ensure DB directory exists.
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0750); err != nil {
		slog.Error("create db dir", "err", err)
		os.Exit(1)
	}

	db, err := NewDB(cfg.DBPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}

	proxy := NewProxyManager(cfg, db)

	// Sync proxy config on startup in case it got out of sync.
	if err := proxy.SyncConfig(); err != nil {
		slog.Warn("initial sync failed", "err", err)
	}

	bot, err := NewBot(cfg, db, proxy)
	if err != nil {
		slog.Error("create bot", "err", err)
		os.Exit(1)
	}

	slog.Info("bot started")
	bot.Run()
}
