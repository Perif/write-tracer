.PHONY: all build clean
 
# List of utility programs
UTILS = echopid test_fork
 
# Default target
all: build
 
# Build target: compiles all utilities
build: $(UTILS)
 
# Generic pattern rule for building Go programs
%: %.go
	@echo "Building $@..."
	go build -o $@ $<
	@echo "Build complete."
 
# Clean target: removes the compiled executables
clean:
	@echo "Cleaning up..."
	@rm -f $(UTILS)
	@echo "Clean complete."
