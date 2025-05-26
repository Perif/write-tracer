# Makefile
.PHONY: generate build clean run caps install

# Generate Go bindings from eBPF C code
generate:
	go generate ./...

# Build the application
build: generate
	go build -o write-tracer

# Setup capabilities (one-time)
caps: build
	@echo "Setting up capabilities..."
	sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_dac_override+ep' ./write-tracer
	@echo "Capabilities set. You can now run without sudo."
	@getcap ./write-tracer

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
	sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_dac_override+ep' /usr/local/bin/write-tracer
	@echo "Installed to /usr/local/bin/write-tracer with capabilities"

# Example usage targets
example-all: caps
	./write-tracer 1234

example-fds: caps
	./write-tracer 1234 1 2