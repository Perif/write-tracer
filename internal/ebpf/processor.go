package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"write-tracer/internal/config"
	"write-tracer/internal/event"
	"write-tracer/internal/output"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

func StartProcessing(ctx context.Context, cfg config.Config, eventsMap, trackedPidsMap *ebpf.Map) error {
	rd, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		return fmt.Errorf("create ring buffer reader: %w", err)
	}

	eventChan := make(chan event.WriteEvent, 1024)

	go processEvents(ctx, cfg, rd, eventChan)
	go countTrackedPids(ctx, cfg.TrackingInterval, trackedPidsMap)
	go readRingBuffer(ctx, rd, eventChan)

	return nil
}

func processEvents(ctx context.Context, cfg config.Config, rd *ringbuf.Reader, eventChan <-chan event.WriteEvent) {
	defer rd.Close()

	fw := output.NewFileWriter(cfg.FileOutput, cfg.MaxRecordsFileOutput)
	defer fw.Close()

	var loki *output.LokiClient
	if cfg.LokiEndpoint != "" {
		loki = output.NewLokiClient(cfg.LokiEndpoint)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-eventChan:
			line := ev.String()
			fmt.Println(line)
			output.IncrementWriteCalls()

			if err := fw.Write(line); err != nil {
				slog.Warn("File write failed", "error", err)
			}

			if loki != nil {
				go func(e event.WriteEvent) {
					if err := loki.Push(e); err != nil {
						slog.Warn("Loki push failed", "error", err)
					}
				}(ev)
			}
		}
	}
}

func countTrackedPids(ctx context.Context, interval time.Duration, trackedPidsMap *ebpf.Map) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count := 0
			iter := trackedPidsMap.Iterate()
			var key, val uint32
			for iter.Next(&key, &val) {
				count++
			}
			output.UpdateTrackedThreads(count)
			slog.Info("Current tracking status", "threads_count", count)
		}
	}
}

func readRingBuffer(ctx context.Context, rd *ringbuf.Reader, eventChan chan<- event.WriteEvent) {
	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			slog.Error("Ring buffer read failed", "error", err)
			continue
		}

		var ev event.WriteEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &ev); err != nil {
			slog.Error("Event parse failed", "error", err)
			continue
		}

		select {
		case eventChan <- ev:
		case <-ctx.Done():
			return
		default:
			slog.Warn("Event channel full, dropping event")
		}
	}
}
