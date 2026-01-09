package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"write-tracer/internal/api"
	"write-tracer/internal/config"
	"write-tracer/internal/ebpf"
	"write-tracer/internal/output"
	"write-tracer/internal/pidmgr"
)

func main() {
	cfg := config.Parse()

	if cfg.MetricsPort > 0 {
		if err := output.StartMetricsServer(cfg.MetricsPort); err != nil {
			slog.Error("Failed to start metrics server", "error", err)
		} else {
			slog.Info("Metrics server started", "port", cfg.MetricsPort)
		}
	}

	if cfg.NumFDs > 0 {
		fds := make([]uint32, cfg.NumFDs)
		copy(fds, cfg.TargetFDs[:cfg.NumFDs])
		slog.Info("Monitoring write calls", "pid", cfg.TargetPID, "file_descriptors", fds)
	} else {
		slog.Info("Monitoring write calls", "pid", cfg.TargetPID, "file_descriptors", "all")
	}

	coll, links, err := ebpf.Load(cfg)
	if err != nil {
		slog.Error("Failed to load eBPF", "error", err)
		os.Exit(1)
	}
	defer coll.Close()
	for _, l := range links {
		defer l.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		slog.Info("Interrupt received")
		cancel()
	}()

	// Initialize PID registry for dynamic tracking
	registry := pidmgr.New(coll.Maps["tracked_pids"], 5*time.Second)
	registry.StartLivenessMonitor(ctx)

	// If a CLI PID was provided, register it in the registry (so liveness monitoring works)
	if cfg.TargetPID != 0 {
		if _, err := registry.RegisterPID(cfg.TargetPID); err != nil {
			// We already initialized it in loader, but we want it in the registry too.
			// Currently loader does it directly. Let's rely on loader for initial setup
			// but RegisterPID will track it for liveness monitoring.
			// Since RegisterPID updates the map again, it's safe (idempotent map update).
			slog.Warn("Failed to register CLI PID with registry", "pid", cfg.TargetPID, "error", err)
		}
	}

	if cfg.RESTPort > 0 {
		server := api.New(registry, cfg.RESTPort)
		if err := server.Start(); err != nil {
			slog.Error("Failed to start REST server", "error", err)
		} else {
			slog.Info("REST API server started", "port", cfg.RESTPort)
		}
	}

	// Update processor to use registry methods if needed, or just let it run.
	// The processor mainly consumes events. The liveness monitor runs separately.

	if err := ebpf.StartProcessing(ctx, cfg, coll.Maps["events"], coll.Maps["tracked_pids"]); err != nil {
		slog.Error("Failed to start processing", "error", err)
		os.Exit(1)
	}

	slog.Info("Tracing write calls... Hit Ctrl-C to stop.")
	<-ctx.Done()
	slog.Info("Shutting down...")
}
