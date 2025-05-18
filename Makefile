.PHONY: build test clean run

# Build variables
BINARY_NAME=schemaregistry
BUILD_DIR=build
CMD_DIR=cmd/schemaregistry

# Go variables
GO=go
GOFMT=gofmt
GOLINT=golangci-lint

# Build the application
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

# Run tests
test:
	@echo "Running tests..."
	@$(GO) test -v ./...

# Run linter
lint:
	@echo "Running linter..."
	@$(GOLINT) run

# Format code
fmt:
	@echo "Formatting code..."
	@$(GOFMT) -w .

# Clean build directory
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

# Run the application
run: build
	@echo "Running..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@$(GO) mod download
	@$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Help
help:
	@echo "Available targets:"
	@echo "  build   - Build the application"
	@echo "  test    - Run tests"
	@echo "  lint    - Run linter"
	@echo "  fmt     - Format code"
	@echo "  clean   - Clean build directory"
	@echo "  run     - Run the application"
	@echo "  deps    - Install dependencies" 