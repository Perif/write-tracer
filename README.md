# Go eBPF Write Tracer - Complete Setup Guide

## Project Structure

Create this directory structure:

```
write-tracer/
├── go.mod
├── go.sum
├── main.go                    # Main Go application
├── write_tracer.bpf.c        # eBPF kernel program
├── Makefile                  # Build automation
├── headers/
│   └── vmlinux.h            # Kernel type definitions
└── generated files (auto-created):
    ├── bpf_bpfeb.go
    ├── bpf_bpfel.go
    └── bpf_bpfeb.o, bpf_bpfel.o
```

## 1. Initialize Go Module

```bash
mkdir write-tracer && cd write-tracer
go mod init write-tracer
```

## 2. Create go.mod

```go
module write-tracer

go 1.21

require github.com/cilium/ebpf v0.12.3

require (
    golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
    golang.org/x/sys v0.15.0 // indirect
)
```

## 3. Get vmlinux.h

You need kernel type definitions. Choose one method:

### Option A: Generate from your kernel
```bash
mkdir headers
# If you have bpftool installed:
bpftool btf dump file /sys/kernel/btf/vmlinux format c > headers/vmlinux.h

# Or use this script:
curl -sf https://raw.githubusercontent.com/aquasecurity/libbpfgo/main/helpers/kernel-config.sh | bash
```

### Option B: Download pre-built (Ubuntu/x86_64)
```bash
mkdir headers
curl -o headers/vmlinux.h https://raw.githubusercontent.com/aquasecurity/tracee/main/3rdparty/btfhub/vmlinux_ubuntu_2204_x86_64.h
```

## 4. Create Makefile

```makefile
# Makefile
.PHONY: generate build clean run

# Generate Go bindings from eBPF C code
generate:
	go generate ./...

# Build the application
build: generate
	go build -o write-tracer

# Clean generated files
clean:
	rm -f write-tracer
	rm -f bpf_*.go bpf_*.o

# Run with example (replace 1234 with actual PID)
run: build
	sudo ./write-tracer 1234

# Install dependencies
deps:
	go mod tidy
	go mod download

# Full build from scratch
all: clean deps generate build

# Development - rebuild on changes
dev:
	find . -name "*.go" -o -name "*.c" | entr -r make build
```

## 5. Build and Run

```bash
# Install dependencies
go mod tidy

# Generate eBPF bindings (this compiles the .bpf.c file)
go generate

# Build the Go application
go build -o write-tracer

# Run (needs root privileges)
sudo ./write-tracer <PID> [fd1] [fd2] ...
```

## Usage Examples

### Monitor all writes for a process:
```bash
# Find your target process
ps aux | grep your_app 

# Monitor all writes
sudo ./write-tracer 1234
```

### Monitor specific file descriptors:
```bash
# Monitor stdout (1) and stderr (2)
sudo ./write-tracer 1234 1 2

# Monitor specific files (check with lsof -p <PID>)
sudo ./write-tracer 1234 3 4 5
```

### Test with shell commands:
```bash
# Terminal 1: Monitor current shell
sudo ./write-tracer $$

# Terminal 2: Generate writes
echo "Hello World"
ls -la > /tmp/test.txt
cat /proc/version
```

## Advantages of Go + Cilium eBPF

### Compared to C approach:
- **Type Safety**: Go's type system catches errors at compile time
- **Memory Management**: No manual memory management
- **Error Handling**: Proper error handling with Go's error interface
- **Concurrency**: Built-in goroutines for handling events
- **Cross Compilation**: Easy to build for different architectures
- **Package Management**: Modern dependency management with go modules

### Code Generation:
- **bpf2go**: Automatically generates Go bindings from C code
- **Type Matching**: Ensures Go structs match eBPF structs
- **Map Abstraction**: High-level map operations
- **Link Management**: Automatic cleanup of eBPF resources

## Sample Output

```
Monitoring write calls for PID 1234 on file descriptors: 1 2 
Successfully started, tracing write calls... Hit Ctrl-C to stop.
[14:32:15.123] PID: 1234, TID: 1234, COMM: bash, FD: 1, BYTES: 12
[14:32:16.456] PID: 1234, TID: 1234, COMM: bash, FD: 2, BYTES: 25
[14:32:17.789] PID: 1234, TID: 1234, COMM: cat, FD: 1, BYTES: 1024
```

## Troubleshooting

### Build Issues:
```bash
# Missing bpf2go
go install github.com/cilium/ebpf/cmd/bpf2go@latest

# Permission issues
sudo chown -R $USER:$USER .

# Missing clang
sudo apt install clang llvm  # Ubuntu
sudo dnf install clang llvm  # Fedora
```

### Runtime Issues:
```bash
# Check if eBPF is enabled
ls /sys/fs/bpf

# Check kernel version (needs 4.15+)
uname -r

# Enable debug output
export EBPF_DEBUG=1
```

### Finding File Descriptors:
```bash
# List all open files for a process
lsof -p <PID>

# Check file descriptors in /proc
ls -la /proc/<PID>/fd/

# Monitor a process and see what files it opens
strace -e openat -p <PID>
```

## Advanced Features

The Go approach makes it easy to add:

- **JSON output** for log processing
- **Metrics export** to Prometheus
- **Database storage** of events
- **Web dashboard** for visualization
- **Filtering by filename** patterns
- **Rate limiting** and sampling
- **Multiple process monitoring**

## Development Workflow

```bash
# Watch for changes and rebuild
make dev

# Or manually:
find . -name "*.go" -o -name "*.c" | entr -r make build
```

This setup gives you a robust, maintainable eBPF application with all the benefits of Go's ecosystem!

################################################################################################
# Running eBPF Programs Without Sudo
################################################################################################
## Method 1: Linux Capabilities (Recommended)

Grant specific capabilities to your binary instead of requiring full root access:

### Set Capabilities on Binary
```bash
# Build your program first
go build -o write-tracer

# Grant required capabilities
sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_sys_admin+ep' ./write-tracer

# Now run without sudo
./write-tracer 1234 1 2
```

### Required Capabilities Explained:
- **CAP_BPF**: Load and manage eBPF programs (kernel 5.8+)
- **CAP_PERFMON**: Access performance monitoring interfaces  
- **CAP_SYS_ADMIN**: Fallback for older kernels, some map operations

### For Older Kernels (< 5.8):
```bash
# Older kernels need different capabilities
sudo setcap 'cap_sys_admin+ep cap_sys_resource+ep' ./write-tracer
```

### Verify Capabilities:
```bash
# Check what capabilities are set
getcap ./write-tracer

# Should show: ./write-tracer = cap_bpf,cap_perfmon,cap_sys_admin+ep
```

## Method 2: Systemd Service with Capabilities

Create a systemd service that runs with minimal privileges:

### Create Service File: `/etc/systemd/system/write-tracer.service`
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

### Enable and Start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable write-tracer
sudo systemctl start write-tracer
sudo systemctl status write-tracer
```

## Method 3: Unprivileged eBPF (Limited)

Some eBPF programs can run unprivileged, but with restrictions:

### Enable Unprivileged eBPF:
```bash
# Check current setting
sysctl kernel.unprivileged_bpf_disabled

# Enable unprivileged eBPF (if disabled)
sudo sysctl -w kernel.unprivileged_bpf_disabled=0

# Make permanent
echo 'kernel.unprivileged_bpf_disabled=0' | sudo tee -a /etc/sysctl.conf
```

### Modified eBPF Program for Unprivileged Mode:
```c
// Unprivileged programs have restrictions:
// - No kernel function calls in some contexts
// - Limited map types
// - No access to arbitrary kernel memory

SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_enter_unpriv(struct trace_event_raw_sys_enter* ctx) {
    // Simplified version for unprivileged mode
    // Some kernel functions may not be available
    
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    
    // Basic filtering and event creation
    // ... rest of the logic
    
    return 0;
}
```

**Note**: Unprivileged eBPF has many limitations and may not work for all tracepoints.

## Method 4: Container Approach

Run in a container with specific capabilities:

### Dockerfile:
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

### Run with Capabilities:
```bash
docker build -t write-tracer .

docker run --rm \
  --cap-add=BPF \
  --cap-add=PERFMON \
  --cap-add=SYS_ADMIN \
  -v /sys:/sys:ro \
  -v /proc:/proc:ro \
  write-tracer 1234
```

## Method 5: User Namespaces + File Capabilities

Combine user namespaces with capabilities:

### Setup Script: `setup-caps.sh`
```bash
#!/bin/bash
set -e

BINARY="./write-tracer"
USER=$(whoami)

echo "Setting up capabilities for eBPF program..."

# Build if needed
if [ ! -f "$BINARY" ]; then
    echo "Building program..."
    go build -o write-tracer
fi

# Set capabilities
echo "Setting capabilities..."
sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_sys_admin+ep' "$BINARY"

# Verify
echo "Verifying capabilities..."
getcap "$BINARY"

echo "Setup complete! You can now run: $BINARY <pid> [fds...]"
echo "Example: $BINARY 1234 1 2"
```

### Usage:
```bash
# One-time setup
chmod +x setup-caps.sh
./setup-caps.sh

# Now run without sudo
./write-tracer 1234 1 2
```

## Method 6: Makefile Integration

Integrate capability setup into your Makefile:

### Enhanced Makefile:
```makefile
.PHONY: generate build clean run setup-caps install

# Build the application
build: generate
	go build -o write-tracer

# Setup capabilities (one-time)
setup-caps: build
	@echo "Setting up capabilities..."
	sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_sys_admin+ep' ./write-tracer
	@echo "Capabilities set. You can now run without sudo."
	@getcap ./write-tracer

# Run without sudo (after setup-caps)
run-nosudo: 
	@if ! getcap ./write-tracer | grep -q cap_bpf; then \
		echo "Capabilities not set. Run 'make setup-caps' first."; \
		exit 1; \
	fi
	./write-tracer $(ARGS)

# Install system-wide with capabilities
install: build
	sudo cp write-tracer /usr/local/bin/
	sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_sys_admin+ep' /usr/local/bin/write-tracer
	@echo "Installed to /usr/local/bin/write-tracer with capabilities"

# Example usage targets
example-all: setup-caps
	./write-tracer 1234

example-fds: setup-caps
	./write-tracer 1234 1 2
```

### Usage:
```bash
# One-time setup
make setup-caps

# Run examples
make example-all ARGS="1234"
make example-fds ARGS="1234 1 2"

# Or directly
make run-nosudo ARGS="1234 1 2"
```

## Security Considerations

### Capability Risks:
- **CAP_SYS_ADMIN** is powerful - consider if you really need it
- **CAP_BPF** is safer but only available on newer kernels
- Always use the minimal set of capabilities needed

### Best Practices:
1. **Drop capabilities** after eBPF program is loaded (if possible)
2. **Use systemd** for production deployments
3. **Container isolation** for additional security
4. **File permissions** - make binary owned by root, executable by group

### Capability Dropping in Code:
```go
// After loading eBPF program, drop capabilities
import "golang.org/x/sys/unix"

func dropCapabilities() error {
    // Drop all capabilities after eBPF setup
    return unix.Prctl(unix.PR_SET_KEEPCAPS, 0, 0, 0, 0)
}
```
