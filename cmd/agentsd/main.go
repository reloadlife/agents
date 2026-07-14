package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reloadlife/agents/internal/api"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/job"
	"github.com/reloadlife/agents/internal/memory"
	"github.com/reloadlife/agents/internal/selfupdate"
	"github.com/reloadlife/agents/internal/session"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		os.Exit(cmdServe(os.Args[2:]))
	case "update":
		os.Exit(cmdUpdate(os.Args[2:]))
	case "version", "-version", "--version":
		fmt.Printf("agentsd %s commit=%s date=%s\n", version, commit, date)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `agentsd — agents control plane daemon

Usage:
  agentsd serve --config PATH
  agentsd update [--check] [--force] [--version TAG] [--all]
  agentsd version

Env:
  AGENTSD_TOKEN   bearer token (or name from config auth.bearer_env)

Docs: https://github.com/reloadlife/agents
`)
}

func cmdUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only check for a newer release")
	force := fs.Bool("force", false, "reinstall even if already on latest")
	all := fs.Bool("all", false, "also update agentsctl if installed next to this binary")
	ver := fs.String("version", "", "install a specific tag (e.g. v0.2.2); default latest")
	_ = fs.Parse(args)

	_, err := selfupdate.Run(selfupdate.Options{
		Current:   version,
		Binary:    "agentsd",
		All:       *all,
		Version:   *ver,
		CheckOnly: *check,
		Force:     *force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "update: %v\n", err)
		return 1
	}
	return 0
}

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "config.example.toml", "path to config.toml")
	_ = fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("config", "err", err)
		return 1
	}
	store, err := job.OpenStore(cfg.JobsDir)
	if err != nil {
		log.Error("store", "err", err)
		return 1
	}
	defer store.Close()

	mgr := job.NewManager(cfg, store, log)
	mgr.Start()
	defer mgr.Stop()

	sess, err := session.NewManager(cfg, log)
	if err != nil {
		log.Error("sessions", "err", err)
		return 1
	}

	var mem *memory.Store
	if cfg.MemoryEnabled() {
		emb := memory.NewHTTPEmbedder(cfg.Memory.EmbedURL, cfg.Memory.EmbedModel, cfg.EmbedAPIKey())
		mem, err = memory.OpenWith(cfg.MemoryDir(), memory.OpenOptions{Embedder: emb})
		if err != nil {
			log.Error("memory", "err", err)
			return 1
		}
		defer mem.Close()
		if emb != nil {
			log.Info("memory embeddings enabled", "model", cfg.Memory.EmbedModel, "url", cfg.Memory.EmbedURL)
		} else {
			log.Info("memory FTS only (set memory.embed_url + embed_model for vectors)")
		}
	}

	srv := api.New(cfg, mgr, sess, mem, log)
	httpSrv := &http.Server{
		Addr:    cfg.Listen,
		Handler: srv.Handler(),
		// Header timeout only — WebSocket PTY sessions are long-lived
		ReadHeaderTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("agentsd listening", "addr", cfg.Listen, "version", version, "workspace", cfg.WorkspaceRoot)
		errCh <- httpSrv.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Error("server", "err", err)
			return 1
		}
	case s := <-sig:
		log.Info("shutdown", "signal", s.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}
	return 0
}
