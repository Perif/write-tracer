.PHONY: all build run clean

# Define the Go source file
SRC = echopid.go

# Define the executable name
EXEC = echopid

# Default target: builds and then runs the program
all: build run

# Build target: compiles the Go program
build:
	@echo "Building $(SRC)..."
	go build -o $(EXEC) $(SRC)
	@echo "Build complete."

# Run target: executes the compiled program
run: build
	@echo "Running $(EXEC)..."
	./$(EXEC)

# Clean target: removes the compiled executable
clean:
	@echo "Cleaning up..."
	@rm -f $(EXEC)
	@echo "Clean complete."
