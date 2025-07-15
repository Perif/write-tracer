// main.go
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"flag"
	"strings"
	"log/slog"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS bpf write_tracer.bpf.c -- -I./headers

const maxFDs = 64
const maxDataSize = 256
const maxExecNameSize = 16

// Config matches the eBPF struct
type Config struct {
	TargetPID uint32
	NumFDs    uint32
	TargetFDs [maxFDs]uint32
}

// WriteEvent matches the eBPF struct
type WriteEvent struct {
	PID       uint32
	TID       uint32
	FD        uint32
	Count     uint64
	Timestamp uint64
	Comm      [maxExecNameSize]byte
	Data      [maxDataSize]byte
}

func (e WriteEvent) String() string {
	// Convert comm to string, removing null bytes
	comm := string(bytes.TrimRight(e.Comm[:], "\x00"))

	// Convert timestamp to readable time
	t := time.Unix(0, int64(e.Timestamp))

	return fmt.Sprintf("[%s] PID: %d, TID: %d, COMM: %s, FD: %d, BYTES: %d",
		t.Format("15:04:05.000"), e.PID, e.TID, comm, e.Count)
}

func getLogLevel() slog.Level {
	levelStr := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		// Default to INFO if not set or invalid
		return slog.LevelInfo
	}
}

// process arguments and return a config
func process_args() Config {
	// Create a new logger with the level determined by the environment variable
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: getLogLevel(),
	}))

	// Set this as the default logger for top-level slog functions
	slog.SetDefault(logger)


	// Define flags
	pidPtr := flag.Int("pid", 0, "Process ID to monitor (required)")
	pidShorthandPtr := flag.Int("p", 0, "Shorthand for --pid")

	// listen to stdout and stderr by default
	fdStringPtr := flag.String("file-descriptors", "", "Comma-separated list of file descriptors to monitor (e.g., '1,2,3'). If empty, all write calls are monitored.")
	fdStringShorthandPtr := flag.String("f", "", "Shorthand for --file-descriptors")

	// lokiEndpointPtr := flag.String("loki-endpoint", "", "URL of the Loki server push endpoint (optional)")
	// lokiEndpointShorthandPtr := flag.Int("l", "", "Shorthand for --loki-endpoint")

	// fileOutputPtr := flag.String("file-output", "", "File to write the captured outputs (optional)")
	// fileOutputShorthandPtr := flag.String("o", "", "Shorthand for --file-output")

	flag.Usage = func() {
		// Keeping fmt for usage message as it typically goes to stdout
		fmt.Printf("Usage: %s --pid <pid> [--file-descriptors <fd1,fd2,...>] [--loki-endpoint <url>]\n", os.Args[0])
		fmt.Printf("       %s -p <pid> [-f <fd1,fd2,...>] [-l <url>]\n\n", os.Args[0])
		fmt.Println("Arguments:")
		flag.PrintDefaults() // Prints default help for all flags
		fmt.Println("\nExamples:")
		fmt.Printf("  %s -p 1234 -f 0,1 -l http://localhost:3100/api/prom/push # Monitor FDs 0,1 for PID 1234 and push to Loki\n", os.Args[0])
		fmt.Printf("  %s --pid 1234 --loki-endpoint http://loki.example.com/api/prom/push # Monitor all write calls for PID 1234 and push to Loki\n", os.Args[0])
		fmt.Printf("  %s -p 5678           # Monitor all write calls for PID 5678 (no Loki push)\n", os.Args[0])
	}

	// Parse command-line arguments
	flag.Parse()
	slog.Debug("Arguments parsed")

	// Determine the actual PID, prioritizing shorthand if both are provided
	targetPID := *pidPtr
	if *pidShorthandPtr != 0 {
		targetPID = *pidShorthandPtr
	}
	slog.Debug("Determined target PID", "pid", targetPID)

	// Validate PID
	if targetPID == 0 {
		slog.Error("PID is required.")
		flag.Usage() // Show usage on error
		os.Exit(1)
	}

	// Determine the actual FD string, prioritizing shorthand if both are provided
	fdString := *fdStringPtr
	if *fdStringShorthandPtr != "" {
		fdString = *fdStringShorthandPtr
	}
	slog.Debug("Determined FD string", "fd_string", fdString)

	// // Determine the actual Loki endpoint, prioritizing shorthand if both are provided
	// lokiEndpoint := *lokiEndpointPtr
	// if *lokiEndpointShorthandPtr != "" {
	// 	lokiEndpoint = *lokiEndpointShorthandPtr
	// }

	// // Determine the actual Loki endpoint, prioritizing shorthand if both are provided
	// fileOutput:= *fileOutputPtr
	// if *fileOutputShorthandPtr != "" {
	// 	fileOutput = *fileOutputShorthandPtr
	// }

	// build the config
	var config Config
	config.TargetPID = uint32(targetPID)
	config.NumFDs = 0 // Initialize NumFDs

	// Parse file descriptors if provided
	if fdString != "" {
		fdParts := strings.Split(fdString, ",")
		for _, part := range fdParts {
			fd, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
			if err != nil {
				slog.Error("Invalid FD", "fd", part, "error", err)
				os.Exit(1)
			}
			if config.NumFDs < maxFDs {
				config.TargetFDs[config.NumFDs] = uint32(fd)
				config.NumFDs++
			} else {
				slog.Warn("Too many file descriptors provided. Monitoring up to %d FDs.", maxFDs)
				break // Stop adding FDs if maxFDs is reached
			}
		}
	}
	slog.Debug("Config built", "config", config)

	return config
}

func loadAndConfigureEBPF(config Config) (*ebpf.Collection, link.Link, error) {
	slog.Debug("Attempting to remove memlock limit")
	// Remove memory limit for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, nil, fmt.Errorf("failed to remove memlock limit: %w", err)
	}
	slog.Debug("Memlock limit removed")

	slog.Debug("Loading eBPF spec")
	// Load pre-compiled programs and maps into the kernel
	spec, err := loadBpf()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load eBPF spec: %w", err)
	}
	slog.Debug("eBPF spec loaded")

	slog.Debug("Creating eBPF collection")
	// create a new ebpf collection
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create eBPF collection: %w", err)
	}
	slog.Debug("eBPF collection created")

	slog.Debug("Updating config map")
	// Update configuration map
	key := uint32(0)
	if err := coll.Maps["config_map"].Update(key, config, ebpf.UpdateAny); err != nil {
		coll.Close()
		return nil, nil, fmt.Errorf("failed to update config map: %w", err)
	}
	slog.Debug("Config map updated")

	slog.Debug("Attaching to tracepoint")
	// Attach to tracepoint
	l, err := link.Tracepoint("syscalls", "sys_enter_write", coll.Programs["trace_write_enter"], nil)
	if err != nil {
		coll.Close()
		return nil, nil, fmt.Errorf("failed to attach tracepoint: %w", err)
	}
	slog.Debug("Tracepoint attached")

	return coll, l, nil
}

func setupSignalHandler(cancel context.CancelFunc) {
	slog.Debug("Setting up signal handler")
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		slog.Info("Interrupt signal received")
		cancel()
	}()
}

func startEventProcessing(ctx context.Context, eventsMap *ebpf.Map) error {
	slog.Debug("Creating ring buffer reader")
	rd, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		return fmt.Errorf("failed to create ring buffer reader: %w", err)
	}
	// Moved defer rd.Close() into the goroutine
	// defer rd.Close()
	slog.Debug("Ring buffer reader created")

	go func() {
		// Ensure the reader is closed when the goroutine exits
		defer rd.Close()
		slog.Debug("Ring buffer reader closing deferred")

		for {
			select {
			case <-ctx.Done():
				slog.Debug("Context cancelled, exiting event processing goroutine")
				return // Exit goroutine when context is cancelled
			default:
				record, err := rd.Read()
				if err != nil {
					if errors.Is(err, ringbuf.ErrClosed) {
						slog.Debug("Ring buffer closed, exiting event processing goroutine")
						return
					}
					slog.Error("Reading from ring buffer failed", "error", err)
					continue
				}

				// Parse event
				var event WriteEvent
				if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
					slog.Error("Failed to parse event", "error", err)
					continue
				}

				fmt.Println(event)

				// Log the event details as info
				slog.Info("Write event",
					"timestamp", time.Unix(0, int64(event.Timestamp)).Format("15:04:05.000"),
					"pid", event.PID,
					"tid", event.TID,
					"comm", string(bytes.TrimRight(event.Comm[:], "\x00")),
					"fd", event.FD,
					"bytes", event.Count,
				)
			}
		}
	}()

	slog.Debug("Started event processing goroutine")
	return nil
}

func main() {
	slog.Debug("Starting application")

	config := process_args()

	// Log configuration details
	fields := []slog.Attr{
		slog.Uint64("pid", uint64(config.TargetPID)),
	}
	if config.NumFDs > 0 {
		var fds []uint32
		// Safely copy FDs up to NumFDs
		fds = make([]uint32, config.NumFDs)
		copy(fds, config.TargetFDs[:config.NumFDs])
		fields = append(fields, slog.Any("file_descriptors", fds))
	} else {
		fields = append(fields, slog.String("file_descriptors", "all"))
	}

	slog.Info("Monitoring write calls", fields)

	coll, link, err := loadAndConfigureEBPF(config)
	if err != nil {
		slog.Error("Failed to load and configure eBPF", "error", err)
		os.Exit(1)
	}
	defer coll.Close()
	defer link.Close()

	ctx, cancel := context.WithCancel(context.Background())
	slog.Debug("Context created")

	setupSignalHandler(cancel)
	slog.Debug("Signal handler setup")

	defer cancel() // Ensure cancel is called on exit

	if err := startEventProcessing(ctx, coll.Maps["events"]); err != nil {
		slog.Error("Failed to start event processing", "error", err)
		os.Exit(1)
	}

	slog.Info("Successfully started, tracing write calls... Hit Ctrl-C to stop.")

	<-ctx.Done()
	slog.Info("Shutting down...")
}