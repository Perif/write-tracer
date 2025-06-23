# Go eBPF Write Tracer 

## 1 - Getting the program to run

To run the code you need to retrieve the header for your kernel. Assuming you aleady have it, you can run the steps below to build and run the code.

### 1.1 - Build and Run

See below how to build and run the write tracer app. 

```bash
# Install dependencies
make deps

# Generate eBPF bindings (this compiles the .bpf.c file)
make generate

# Build the Go application
make build

# Run (needs root privileges)
sudo ./write-tracer -p <PID> -f [fd1],[fd2] ...
```

### 1.2 - Get vmlinux.h

You need kernel type definitions. Pick one option:

#### Option A: Generate from your kernel
```bash
mkdir headers
# If you have bpftool installed:
bpftool btf dump file /sys/kernel/btf/vmlinux format c > headers/vmlinux.h

# Or use this script:
curl -sf https://raw.githubusercontent.com/aquasecurity/libbpfgo/main/helpers/kernel-config.sh | bash
```

#### Option B: Download pre-built (Ubuntu/x86_64)
```bash
mkdir headers
curl -o headers/vmlinux.h https://raw.githubusercontent.com/aquasecurity/tracee/main/3rdparty/btfhub/vmlinux_ubuntu_2204_x86_64.h
```

## 3 - Test the tool

Use the echopid utility:
```bash
# Find your target process
cd utilities && make && ./echopid
```

Run the tracer on all file-descriptors, assuming that echopid gave PID `441368`:

```bash
./write-tracer -p 441368
```
To trace only file descriptors 0 and 1:

```bash
./write-tracer -p 441368 -f 0,1
```


## 4 - Running as a deamon

To run the code as a deamon you can pick one of the two options below.

### 4.1 - Option 1 : Systemd Service with Capabilities

Create a systemd service that runs with minimal privileges:

#### Create Service File: `/etc/systemd/system/write-tracer.service`
```ini
[Unit]
Description=eBPF Write Tracer
After=network.target

[Service]
Type=simple
User=your-username
Group=your-group
ExecStart=/path/to/write-tracer 1234
Restart=always
RestartSec=5

# Grant only required capabilities
CapabilityBoundingSet=CAP_BPF CAP_PERFMON CAP_SYS_ADMIN
AmbientCapabilities=CAP_BPF CAP_PERFMON CAP_SYS_ADMIN

# Security hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/log

[Install]
WantedBy=multi-user.target
```

#### Enable and Start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable write-tracer
sudo systemctl start write-tracer
sudo systemctl status write-tracer
```
### 4.2 Option 2: Container Approach

Run in a container with specific capabilities:

#### Dockerfile:
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o write-tracer

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/write-tracer .
CMD ["./write-tracer"]
```

#### Run with Capabilities:
```bash
docker build -t write-tracer .

docker run --rm \
  --cap-add=BPF \
  --cap-add=PERFMON \
  --cap-add=SYS_ADMIN \
  -v /sys:/sys:ro \
  -v /proc:/proc:ro \
  write-tracer -p 1234
```

## 5. Debug ebpf

To print debug information in the epbf code (`write-tracer.pbf.c`) you need to genreate a debug go bpf code:

```bash
make generate-debug
```

Then when running the application, open a terminal and run the following:

```bash
sudo cat /sys/kernel/debug/tracing/trace_pipe
```