package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"tg-proxy/internal/bot"
	"tg-proxy/internal/config"
	"tg-proxy/internal/db"
	"tg-proxy/internal/proxy"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0750); err != nil {
		slog.Error("create db dir", "err", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}

	pm := proxy.NewManager(cfg, database)

	if err := pm.SyncConfig(); err != nil {
		slog.Warn("initial sync failed", "err", err)
	}

	b, err := bot.New(cfg, database, pm)
	if err != nil {
		slog.Error("create bot", "err", err)
		os.Exit(1)
	}

	slog.Info("bot started")
	b.Run()
}
