package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"write-tracer/internal/config"
	"write-tracer/internal/ebpf"
	"write-tracer/internal/output"
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

	if err := ebpf.StartProcessing(ctx, cfg, coll.Maps["events"], coll.Maps["tracked_pids"]); err != nil {
		slog.Error("Failed to start processing", "error", err)
		os.Exit(1)
	}

	slog.Info("Tracing write calls... Hit Ctrl-C to stop.")
	<-ctx.Done()
	slog.Info("Shutting down...")
}
