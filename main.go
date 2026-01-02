package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/engine"
	"n9e-alter-service/internal/ingest"
	"n9e-alter-service/internal/redismgr"
	"n9e-alter-service/internal/rulesrepo"
	"n9e-alter-service/internal/runtime"
	"n9e-alter-service/internal/server"
	"n9e-alter-service/internal/state"
	"n9e-alter-service/internal/telemetry"
	"n9e-alter-service/internal/workers"
)

func main() {
	var cfgPath string
	var addr string
	var webDir string
	var dataDir string

	flag.StringVar(&cfgPath, "config", "", "config file path (json)")
	flag.StringVar(&addr, "addr", "", "listen address, override config")
	flag.StringVar(&webDir, "web-dir", "", "web dir, override config")
	flag.StringVar(&dataDir, "data-dir", "", "data dir, override config")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if addr != "" {
		cfg.Addr = addr
	}
	if webDir != "" {
		cfg.WebDir = webDir
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
	}

	st := state.New()
	if err := st.LoadFromFile(cfg.State.SnapshotFile); err != nil {
		log.Printf("load snapshot: %v", err)
	}

	stats := telemetry.New()

	eng, err := engine.New(cfg, st)
	if err != nil {
		log.Fatalf("init engine: %v", err)
	}

	repo := rulesrepo.New(cfg.DataDir)
	if rs, _, ok, err := repo.LoadCurrent(); err != nil {
		log.Printf("load current rules: %v", err)
	} else if ok {
		cfg = rulesrepo.ApplyToConfig(cfg, rs)
		eng2, err := engine.New(cfg, st)
		if err != nil {
			log.Printf("init engine from current rules: %v", err)
		} else {
			eng = eng2
		}
	}

	rt := runtime.New(runtime.Snapshot{Cfg: cfg, Eng: eng})

	// Redis 仅用于高并发 dedup/聚合热状态（不用于持久化存储）。
	rm := redismgr.New(cfg.State.Redis)
	ing := ingest.New(cfg.Push, cfg.State, eng, st, rm, stats)

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	ing.Start(rootCtx)

	startPullLoopRuntime(rootCtx, cfg, rt)
	startSnapshotLoop(rootCtx, cfg, st)

	notify := workers.NewNotifierWithRuntime(rt, st, rm, stats)
	notify.Start(rootCtx)

	daily := workers.NewDailyReporterWithRuntime(rt, st)
	daily.Start(rootCtx)

	s := server.NewWithRuntime(rt, repo, st, ing, rm, stats)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", cfg.Addr)
		errCh <- httpSrv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("signal: %s", sig.String())
		cancelRoot()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := st.SaveToFile(cfg.State.SnapshotFile); err != nil {
		log.Printf("save snapshot: %v", err)
	}
	if rm != nil {
		rm.Close()
	}

	listenErr := <-errCh
	if listenErr != nil && listenErr != http.ErrServerClosed {
		log.Printf("listen: %v", listenErr)
	}
}

func startPullLoop(ctx context.Context, cfg config.Config, eng *engine.Engine) {
	interval := time.Duration(cfg.Pull.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				_, err := eng.RunOnce(pullCtx)
				cancel()
				if err != nil {
					log.Printf("pull error: %v", err)
				}
			}
		}
	}()
}

func startPullLoopRuntime(ctx context.Context, cfg config.Config, rt *runtime.Runtime) {
	interval := time.Duration(cfg.Pull.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				snap := rt.Get()
				if snap.Eng == nil {
					continue
				}
				if strings.TrimSpace(snap.Cfg.N9E.BaseURL) == "" {
					log.Printf("pull skipped: n9e.base_url is empty")
					continue
				}
				pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				_, err := snap.Eng.RunOnce(pullCtx)
				cancel()
				if err != nil {
					log.Printf("pull error: %v", err)
				}
			}
		}
	}()
}

func startSnapshotLoop(ctx context.Context, cfg config.Config, st *state.Store) {
	interval := time.Duration(cfg.State.SnapshotIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := st.SaveToFile(cfg.State.SnapshotFile); err != nil {
					log.Printf("save snapshot: %v", err)
				}
			}
		}
	}()
}
