# Variables
BINARY_NAME=bin/maximus
MAIN_PKG=./cmd/maximux

# ==============================================================================
# Main Targets
# ==============================================================================

# Default target: builds the binary
all: build

# Build the binary into the bin/ directory (CGO_ENABLED=1 required for sqlite3)
build:
	@echo "  >  Building $(BINARY_NAME)..."
	@CGO_ENABLED=1 go build -o $(BINARY_NAME) $(MAIN_PKG)
	@echo "  >  Build complete!"

# Run the compiled binary directly
exec: build
	@./$(BINARY_NAME)

# ==============================================================================
# Development / Running
# ==============================================================================

# Run the app with go run (no binary creation)
run:
	@CGO_ENABLED=1 go run $(MAIN_PKG) $(args)

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

# Vet all packages
vet:
	@go vet ./...

# ==============================================================================
# .PHONY ensures Make doesn't confuse these targets with file names
.PHONY: all build exec run clean tidy vet