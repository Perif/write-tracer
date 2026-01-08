# Go eBPF Write Tracer 

Trace write syscalls for a process and its children using eBPF.

## Features

- **Thread Tracking**: Automatically tracks all threads and child processes
- **JSON Output**: Events exported as JSON to stdout, file, and/or Loki
- **File Rotation**: Configurable rotation based on record count
- **Prometheus Metrics**: Exposes `write_tracer_tracked_threads` and `write_tracer_write_calls_total`

## Build

```bash
make deps      # Install dependencies
make build     # Build the application
```

### Prerequisites

You need kernel type definitions. Generate from your kernel:
```bash
mkdir -p bpf/headers
bpftool btf dump file /sys/kernel/btf/vmlinux format c > bpf/headers/vmlinux.h
```

## Usage

```bash
sudo ./write-tracer -p <PID> [options]
```

### Options

| Flag | Short | Description |
|------|-------|-------------|
| `--pid` | `-p` | Target PID (required) |
| `--file-output` | `-o` | Output file path |
| `--file-descriptors` | `-f` | Comma-separated FDs to filter |
| `--tracking-interval` | `-i` | Status update interval (seconds) |
| `--max-records-fileoutput` | `-n` | Records per file before rotation |
| `--loki-endpoint` | `-l` | Loki push endpoint URL |
| `--metrics-port` | | Prometheus metrics port (default 2112) |

### Examples

```bash
# Trace all writes for PID 1234
sudo ./write-tracer -p 1234

# Filter to stdout/stderr only, write to file
sudo ./write-tracer -p 1234 -f 1,2 -o /tmp/trace.log

# With Prometheus metrics on port 9100
sudo ./write-tracer -p 1234 --metrics-port 9100
```

## Prometheus Metrics

```bash
curl http://localhost:2112/metrics | grep write_tracer
```

- `write_tracer_tracked_threads` — current thread count
- `write_tracer_write_calls_total` — total captured write calls

## Project Structure

```
write-tracer/
├── cmd/tracer/           # Entry point
├── internal/
│   ├── config/           # CLI flag parsing
│   ├── ebpf/             # eBPF loading and event processing
│   ├── event/            # WriteEvent struct
│   └── output/           # File, Loki, and Prometheus output
├── bpf/                  # eBPF C source and headers
└── utilities/            # Test utilities
```

## Testing

```bash
cd utilities && go build -o echopid echopid.go && ./echopid &
sudo ./write-tracer -p $!
```

## Debug eBPF

```bash
make generate-debug
sudo cat /sys/kernel/debug/tracing/trace_pipe
```

## Running as a Daemon

### Systemd Service

Create `/etc/systemd/system/write-tracer.service`:
```ini
[Unit]
Description=eBPF Write Tracer
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/write-tracer -p <TARGET_PID>
Restart=always
RestartSec=5

CapabilityBoundingSet=CAP_BPF CAP_PERFMON CAP_SYS_ADMIN
AmbientCapabilities=CAP_BPF CAP_PERFMON CAP_SYS_ADMIN

NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable write-tracer
sudo systemctl start write-tracer
```