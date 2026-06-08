.PHONY: all build test clean install

# Binary names
BINARY_NAME=codemap
BENCHMARK_NAME=benchmark

# Build directory
BUILD_DIR=bin

all: build test

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/codemap
	@echo "Building $(BENCHMARK_NAME)..."
	go build -o $(BUILD_DIR)/$(BENCHMARK_NAME) ./cmd/benchmark

test:
	@echo "Running tests..."
	go test ./...

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

# Help target
help:
	@echo "Makefile for Codemap"
	@echo ""
	@echo "Usage:"
	@echo "  make build    - Build the codemap and benchmark binaries"
	@echo "  make test     - Run all tests"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make install  - Install the codemap binary to /usr/local/bin"
	@echo "  make all      - Build and test the project"
