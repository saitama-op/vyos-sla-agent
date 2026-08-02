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
	"time"

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

	// 8. Watchdog: Restart if ALL WAN links go DOWN
	go func() {
		// Give the agent a 60-second grace period on startup before checking
		// so it has time to run initial probes and establish state.
		select {
		case <-time.After(60 * time.Second):
		case <-ctx.Done():
			return // Exit if shutting down during grace period
		}

		// Check the status every 10 seconds
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return // Clean exit if context is cancelled
			case <-ticker.C:
				allDown := true

				// Loop through all configured ISPs
				for _, engine := range engines {
					if engine.CurrentState != decision.StateDown {
						allDown = false
						break
					}
				}

				// If we have engines configured, and EVERY single one is DOWN
				if len(engines) > 0 && allDown {
					slog.Error("CRITICAL: All WAN interfaces are DOWN! Forcing process restart to refresh sockets/state...")
					os.Exit(1) // Systemd will catch this and restart the process
				}
			}
		}
	}()

	// 9. Handle Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("Received shutdown signal, terminating workers gracefully...", "signal", sig)

	cancel()
	wg.Wait()

	slog.Info("Shutdown complete. Agent exited.")
}
