package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qnap-monitor/backend/internal/alert"
	"github.com/qnap-monitor/backend/internal/api"
	"github.com/qnap-monitor/backend/internal/collector"
	"github.com/qnap-monitor/backend/internal/config"
	"github.com/qnap-monitor/backend/internal/store"
)

func main() {
	var (
		addr    = flag.String("addr", envOr("ADDR", ":8080"), "HTTP listen address")
		dataDir = flag.String("data", envOr("DATA_DIR", "./data"), "directory for SQLite + key files")
	)
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		log.Fatalf("mkdir data dir: %v", err)
	}
	keyPath := filepath.Join(*dataDir, "key.bin")
	dbPath := filepath.Join(*dataDir, "monitor.db")

	key, err := config.LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf("load key: %v", err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	cm, err := config.NewManager(st.DB(), key)
	if err != nil {
		log.Fatalf("config manager: %v", err)
	}
	cur := cm.Current()
	if cur.CollectIntervalSeconds == 0 {
		// brand-new DB initial row may be 0; backfill defaults
		zero := 10
		threshold := 55.0
		cpuThreshold := 75.0
		retention := 30
		_, err := cm.Apply(context.Background(), config.Update{
			CollectIntervalSeconds:   &zero,
			TempThresholdCelsius:     &threshold,
			DiskTempThresholdCelsius: &threshold,
			CPUTempThresholdCelsius:  &cpuThreshold,
			RetentionDays:            &retention,
		})
		if err != nil {
			log.Fatalf("seed default config: %v", err)
		}
	}

	am := alert.NewManager(st, cm.Current().TempThresholdCelsius, cm.Current().DiskTempThresholdCelsius, cm.Current().CPUTempThresholdCelsius, cm.Current().WeComWebhookURL)
	if err := am.RestoreFromDB(context.Background()); err != nil {
		log.Printf("warning: restore open alerts: %v", err)
	}
	cl := collector.New(st, cm, am)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cl.Run(ctx)

	staticFs, _ := staticFS()
	srv := &api.Server{
		Store:     st,
		Config:    cm,
		Alerts:    am,
		Collector: cl,
		StaticFS:  staticFs,
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("qnap-monitor listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = httpSrv.Shutdown(shutCtx)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
