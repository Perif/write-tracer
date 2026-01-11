.PHONY: all build clean slurm-plugin mpi-example nccl-example
 
# List of utility programs
UTILS = echopid test_fork
 
# Compiler settings
CC = gcc
MPICC ?= mpicc
NVCC ?= nvcc
CFLAGS = -Wall -O2
LIBS = -lcurl
 
# Slurm specific settings
SLURM_CFLAGS = $(shell pkg-config --cflags slurm 2>/dev/null || echo "-I/usr/include/slurm")
# Note: SPANK plugins are shared objects
 
# Default target
all: build
 
# Build target: compiles all utilities
# We don't verify dependencies here to avoid build failures on machines without Slurm/MPI/NCCL
build: $(UTILS)
	@echo "Standard utilities built. To build optional components, run:"
	@echo "  make slurm-plugin   (Requires Slurm dev headers, libcurl)"
	@echo "  make mpi-example    (Requires MPI, libcurl)"
	@echo "  make nccl-example   (Requires NCCL, MPI, CUDA, libcurl)"
 
# Standard utilities
echopid: echopid.go
	@echo "Building $@..."
	go build -o $@ $<
	@echo "Build complete."

test_fork: test_fork.go
	@echo "Building $@..."
	go build -o $@ $<
	@echo "Build complete."
 
# Optional: Slurm SPANK Plugin
slurm-plugin: slurm_write_tracer.so
 
slurm_write_tracer.so: slurm_write_tracer.c
	@echo "Building Slurm SPANK plugin..."
	$(CC) $(CFLAGS) -fPIC -shared -o $@ $< $(SLURM_CFLAGS) $(LIBS)
	@echo "SPANK plugin built: $@"
 
# Optional: MPI Example
mpi-example: mpi_example
 
mpi_example: mpi_example.c
	@echo "Building MPI example..."
	$(MPICC) $(CFLAGS) -o $@ $< $(LIBS)
	@echo "MPI example built: $@"
 
# Optional: NCCL Example
nccl-example: nccl_example
 
nccl_example: nccl_example.c
	@echo "Building NCCL example..."
	$(NVCC) -o $@ $< $(LIBS) -lmpi -lnccl
	@echo "NCCL example built: $@"
 
# Clean target: removes the compiled executables
clean:
	@echo "Cleaning up..."
	@rm -f $(UTILS) slurm_write_tracer.so mpi_example nccl_example *.o rank_*_output.dat
	@echo "Clean complete."
