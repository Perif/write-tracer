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

## Configuration

- `--pid <PID>`: Process ID to monitor (required unless `--rest-port` is used).
- `--rest-port <port>`: Enable REST API for dynamic PID registration (default: disabled).
- `--metrics-port <port>`: Port for Prometheus metrics (default: 2112).
- `--loki-endpoint <URL>`: URL of Loki server to push logs.
- `--tracking-interval <seconds>`: Interval to update tracked threads (default: 5).
- `--no-stdout`: Deactivate logging to stdout (shorthand `-q`).

## REST API

When `--rest-port` is enabled (e.g., `--rest-port 9092`), you can dynamically manage tracked PIDs.

**Endpoints:**
- `POST /pids`: Register a PID `{"pid": 12345}`
- `DELETE /pids/<pid>`: Unregister a PID
- `GET /pids`: List tracked PIDs

The tracer automatically stops tracking a PID when its process terminates.

**Usage Examples:**

1. **Register a PID:**
    ```bash
    curl -X POST http://127.0.0.1:9092/pids \
         -H "Content-Type: application/json" \
         -d '{"pid": 12345}'
    ```

2. **List tracked PIDs:**
    ```bash
    curl http://127.0.0.1:9092/pids
    ```

3. **Unregister a PID:**
    ```bash
    curl -X DELETE http://127.0.0.1:9092/pids/12345
    ```

## Output

The tracer generates logs in the following format:

### Examples

```bash
# Trace all writes for PID 1234
sudo ./write-tracer -p 1234

# Use REST API for dynamic PID registration (no initial PID)
sudo ./write-tracer --rest-port 9092

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
│   ├── api/              # REST API server
│   ├── config/           # CLI flag parsing
│   ├── ebpf/             # eBPF loading and event processing
│   ├── event/            # WriteEvent struct
│   ├── output/           # File, Loki, and Prometheus output
│   └── pidmgr/           # PID tracking registry
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