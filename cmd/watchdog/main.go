package main

import (
	"context"
	"database/sql"
	"flag"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ysqss/watchdog/internal/alerter"
	"github.com/ysqss/watchdog/internal/api"
	"github.com/ysqss/watchdog/internal/config"
	"github.com/ysqss/watchdog/internal/scheduler"
	"github.com/ysqss/watchdog/internal/store"
)

var (
	configPath = flag.String("config", "config.yaml", "path to config file")
)

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfgMgr, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	cfg := cfgMgr.Get()

	if err := os.MkdirAll("data", 0755); err != nil {
		slog.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	st, err := store.New(db)
	if err != nil {
		slog.Error("failed to init store", "error", err)
		os.Exit(1)
	}

	notifierClient := alerter.NewNotifierClient(cfg.Notifier.BaseURL, cfg.Notifier.Token)
	al := alerter.New(notifierClient, st, cfg.Watchdog.TLSExpiryWarningDays)

	sched := scheduler.New(st, al.OnStateChange, al.OnProbeResult)
	sched.Start()

	targets, err := st.ListTargets()
	if err != nil {
		slog.Error("failed to list targets", "error", err)
		os.Exit(1)
	}
	for _, t := range targets {
		sched.AddTarget(t)
		slog.Info("loaded target", "id", t.ID, "name", t.Name, "type", string(t.Type))
	}

	for _, t := range cfg.Targets {
		existing, _ := st.GetTargetByID(t.ID)
		if existing != nil {
			continue
		}
		if err := st.InsertTarget(&t); err != nil {
			slog.Error("failed to insert target from config", "id", t.ID, "error", err)
			continue
		}
		sched.AddTarget(&t)
		slog.Info("loaded target from config", "id", t.ID, "name", t.Name)
	}

	go startHeartbeat(cfg, notifierClient)
	go startCleanup(cfg, st)

	server := api.NewServer(st, cfgMgr, sched, al)
	handler := api.ApplyMiddleware(server.Handler(), cfgMgr)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", handler)

	httpServer := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("watchdog server starting", "addr", cfg.Server.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	if cfg.Server.DebugPort != "" {
		go func() {
			pprofMux := http.NewServeMux()
			pprofMux.Handle("/debug/pprof/", http.DefaultServeMux)
			pprofServer := &http.Server{Addr: cfg.Server.DebugPort, Handler: pprofMux}
			slog.Info("pprof server starting", "addr", cfg.Server.DebugPort)
			if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("pprof server error", "error", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		switch sig {
		case syscall.SIGHUP:
			slog.Info("received SIGHUP, reloading config")
			if err := cfgMgr.Reload(); err != nil {
				slog.Error("failed to reload config", "error", err)
			} else {
				slog.Info("config reloaded successfully")
				targets, err := st.ListTargets()
				if err != nil {
					slog.Error("failed to list targets for sync", "error", err)
				} else {
					sched.SyncTargets(targets)
					slog.Info("scheduler targets synced", "count", len(targets))
				}
			}
		case syscall.SIGINT, syscall.SIGTERM:
			slog.Info("shutting down", "signal", sig.String())

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

			slog.Info("stopping scheduler...")
			sched.Shutdown()

			slog.Info("flushing pending alerts...")
			al.Shutdown()

			httpServer.SetKeepAlivesEnabled(false)
			if err := httpServer.Shutdown(ctx); err != nil {
				slog.Error("http server shutdown error", "error", err)
			}
			cancel()

			if err := db.Close(); err != nil {
				slog.Error("database close error", "error", err)
			}

			slog.Info("watchdog stopped")
			return
		}
	}
}

func startHeartbeat(cfg *config.Config, client *alerter.NotifierClient) {
	interval := cfg.Watchdog.HeartbeatDuration()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		msg := &alerter.AlertMessage{
			Title:     "Watchdog 心跳",
			Content:   "Watchdog 服务正常运行中",
			Level:     alerter.AlertLevelInfo,
			Service:   "watchdog",
			Timestamp: time.Now(),
		}
		if err := client.Send(ctx, msg); err != nil {
			slog.Error("heartbeat failed", "error", err)
		}
		cancel()
	}
}

func startCleanup(cfg *config.Config, st *store.Store) {
	interval := cfg.Watchdog.CleanupDuration()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := st.Cleanup(cfg.Watchdog.ProbeHistoryDays, cfg.Watchdog.EventHistoryDays); err != nil {
			slog.Error("cleanup failed", "error", err)
		} else {
			slog.Info("cleanup completed",
				"probe_days", cfg.Watchdog.ProbeHistoryDays,
				"event_days", cfg.Watchdog.EventHistoryDays,
			)
		}
	}
}
