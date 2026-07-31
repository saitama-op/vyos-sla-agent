package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	
	"github.com/saitama-op/vyos-sla-agent/internal/config"
	"github.com/saitama-op/vyos-sla-agent/internal/decision"
	"github.com/saitama-op/vyos-sla-agent/internal/exporter"
	"github.com/saitama-op/vyos-sla-agent/internal/probe"
	"github.com/saitama-op/vyos-sla-agent/internal/server"
	"github.com/saitama-op/vyos-sla-agent/internal/vyos"
)

func main() {
	// 1. Define Command-Line Flags
	metricsAddr := flag.String("metrics-bind", ":9101", "Address to bind the Prometheus metrics server")
	apiAddr := flag.String("api-bind", ":8080", "Address to bind the REST API server")
	configPath := flag.String("config", "configs/config.yaml", "Path to the configuration file")
	flag.Parse()

	// 2. Initialize structured JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 3. Load Configuration using the flag
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	slog.Info("Configuration loaded successfully", "wans", len(cfg.WANs), "config_path", *configPath)

	// 4. Initialize Global Components
	exporter.InitPrometheus()
	vyosCtrl := vyos.NewController()
	engines := make(map[string]*decision.Engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5. Start Prometheus Exporter using the flag
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		slog.Info("Starting Prometheus exporter", "address", *metricsAddr)
		if err := http.ListenAndServe(*metricsAddr, nil); err != nil {
			slog.Error("Prometheus exporter failed", "error", err)
		}
	}()

	// 6. Initialize and Start WAN Workers
	var wg sync.WaitGroup

	for _, wan := range cfg.WANs {
		engine := decision.NewEngine(wan.Name)
		engines[wan.Name] = engine

		// Initialize worker without passing a shared connection
		worker := probe.NewWorker(wan, engine, vyosCtrl)
		
		wg.Add(1)
		go func(w *probe.Worker) {
			defer wg.Done()
			w.Run(ctx, cfg.Interval)
		}(worker)
	}

	// 7. Start the HTTP REST API using the flag
	apiServer := server.NewAPIServer(*apiAddr, engines)
	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server failed", "error", err)
		}
	}()

	// 8. Handle Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("Received shutdown signal, terminating workers gracefully...", "signal", sig)

	cancel()
	wg.Wait()

	slog.Info("Shutdown complete. Agent exited.")
}
