# Variables
BINARY_NAME=bin/maximus
MAIN_FILE=main.go

# ==============================================================================
# Main Targets
# ==============================================================================

# Default target: builds the binary
all: build

# Build the binary into the current directory
build:
	@echo "  >  Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) $(MAIN_FILE)
	@echo "  >  Build complete!"

# Run the compiled binary directly (triggers help by default)
exec: build
	@./$(BINARY_NAME)

# ==============================================================================
# Development / Running
# ==============================================================================

# Run 'maximus brew' using go run (no binary creation)
run-brew:
	@go run $(MAIN_FILE) brew

# Run 'maximus help' using go run
run-help:
	@go run $(MAIN_FILE) help

# Run with arbitrary arguments (usage: make run args="some command")
run:
	@go run $(MAIN_FILE) $(args)

# ==============================================================================
# Maintenance
# ==============================================================================

# Clean up binaries and temporary files
clean:
	@echo "  >  Cleaning build cache..."
	@go clean
	@rm -f $(BINARY_NAME)

# Format code and tidy dependencies
tidy:
	@go fmt ./...
	@go mod tidy

# ==============================================================================
# .PHONY ensures Make doesn't confuse these targets with file names
.PHONY: all build exec run-brew run-help run clean tidy