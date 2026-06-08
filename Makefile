.PHONY: start build clean install-deps

# Default target
all: build

# Install dependencies for both backend and frontend
install-deps:
	go mod download
	cd frontend && npm install

# Build the project
build: install-deps
	go build -o bin/codemap ./cmd/codemap
	cd frontend && npm run build

# Start both backend and frontend concurrently
# Using trap to ensure both background processes are killed on Ctrl+C
start:
	@echo "Starting Codemap Guided Learning Platform..."
	@bash -c "trap 'kill 0' EXIT; \
		go run cmd/codemap/main.go serve . & \
		(cd frontend && npm run dev -- --port 5173) & \
		wait"

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf frontend/dist
	rm -rf frontend/node_modules
