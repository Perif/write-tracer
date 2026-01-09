package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	MaxFDs          = 64
	MaxDataSize     = 256
	MaxExecNameSize = 16
)

type Config struct {
	TargetPID            uint32
	NumFDs               uint32
	TargetFDs            [MaxFDs]uint32
	LokiEndpoint         string
	FileOutput           string
	TrackingInterval     time.Duration
	MaxRecordsFileOutput int
	MetricsPort          int
	RESTPort             int
	SilenceStdout        bool
}

func Parse() Config {
	initLogger()

	pidPtr := flag.Int("pid", 0, "Process ID to monitor (required)")
	pidShorthandPtr := flag.Int("p", 0, "Shorthand for --pid")

	fdStringPtr := flag.String("file-descriptors", "", "Comma-separated list of file descriptors to monitor")
	fdStringShorthandPtr := flag.String("f", "", "Shorthand for --file-descriptors")

	lokiEndpointPtr := flag.String("loki-endpoint", "", "URL of the Loki server push endpoint")
	lokiEndpointShorthandPtr := flag.String("l", "", "Shorthand for --loki-endpoint")

	fileOutputPtr := flag.String("file-output", "", "File to write captured outputs")
	fileOutputShorthandPtr := flag.String("o", "", "Shorthand for --file-output")

	trackingIntervalPtr := flag.Int("tracking-interval", 5, "Interval in seconds for tracking status updates")
	trackingIntervalShorthandPtr := flag.Int("i", 5, "Shorthand for --tracking-interval")

	maxRecordsPtr := flag.Int("max-records-fileoutput", 1000, "Maximum records per file before rotation")
	maxRecordsShorthandPtr := flag.Int("n", 0, "Shorthand for --max-records-fileoutput")

	metricsPortPtr := flag.Int("metrics-port", 2112, "Port for Prometheus metrics endpoint (0 to disable)")

	restPortPtr := flag.Int("rest-port", 9092, "Port for REST API endpoint (0 to disable)")
	restPortShorthandPtr := flag.Int("r", 0, "Shorthand for --rest-port")

	silenceStdoutPtr := flag.Bool("no-stdout", false, "Deactivate logging to stdout")
	silenceStdoutShorthandPtr := flag.Bool("q", false, "Shorthand for --no-stdout")

	flag.Usage = func() {
		fmt.Printf("Usage: %s --pid <pid> [options]\n\n", os.Args[0])
		fmt.Println("Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	targetPID := coalesce(*pidShorthandPtr, *pidPtr)
	restPort := coalesce(*restPortShorthandPtr, *restPortPtr)

	// PID is optional if REST mode is enabled (REST can register PIDs dynamically)
	if targetPID == 0 && restPort == 0 {
		slog.Error("PID is required (or enable REST API with --rest-port)")
		flag.Usage()
		os.Exit(1)
	}

	fdString := coalesceStr(*fdStringShorthandPtr, *fdStringPtr)
	lokiEndpoint := coalesceStr(*lokiEndpointShorthandPtr, *lokiEndpointPtr)
	fileOutput := coalesceStr(*fileOutputShorthandPtr, *fileOutputPtr)
	trackingInterval := coalesce(*trackingIntervalShorthandPtr, *trackingIntervalPtr)
	if trackingInterval == 0 {
		trackingInterval = 5
	}
	maxRecords := coalesce(*maxRecordsShorthandPtr, *maxRecordsPtr)
	if maxRecords == 0 {
		maxRecords = 1000
	}

	cfg := Config{
		TargetPID:            uint32(targetPID),
		LokiEndpoint:         lokiEndpoint,
		FileOutput:           fileOutput,
		TrackingInterval:     time.Duration(trackingInterval) * time.Second,
		MaxRecordsFileOutput: maxRecords,
		MetricsPort:          *metricsPortPtr,
		RESTPort:             restPort,
		SilenceStdout:        *silenceStdoutPtr || *silenceStdoutShorthandPtr,
	}

	if fdString != "" {
		for _, part := range strings.Split(fdString, ",") {
			fd, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
			if err != nil {
				slog.Error("Invalid FD", "fd", part, "error", err)
				os.Exit(1)
			}
			if cfg.NumFDs < MaxFDs {
				cfg.TargetFDs[cfg.NumFDs] = uint32(fd)
				cfg.NumFDs++
			}
		}
	}

	return cfg
}

func initLogger() {
	level := slog.LevelInfo
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
}

func coalesce(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
