package output

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var trackedThreads = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "write_tracer_tracked_threads",
	Help: "Number of threads currently being tracked",
})

var writeCalls = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "write_tracer_write_calls_total",
	Help: "Total number of write calls captured",
})

func init() {
	prometheus.MustRegister(trackedThreads)
	prometheus.MustRegister(writeCalls)
}

func UpdateTrackedThreads(count int) {
	trackedThreads.Set(float64(count))
}

func IncrementWriteCalls() {
	writeCalls.Inc()
}

func StartMetricsServer(port int) error {
	if port <= 0 {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("Metrics server failed", "error", err)
		}
	}()
	return nil
}
