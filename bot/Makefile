.PHONY: build run clean deps

BINARY_NAME=musicbot
BUILD_DIR=build
CONFIG_FILE=config.json

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/bot

run: build
	@echo "Starting $(BINARY_NAME)..."
	cd $(BUILD_DIR) && ./$(BINARY_NAME) -config ../$(CONFIG_FILE)

deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

clean:
	@echo "Cleaning build directory..."
	rm -rf $(BUILD_DIR)
	rm -f bot.db

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

test:
	@echo "Running tests..."
	go test ./...

dev: deps build
	@echo "Starting in development mode..."
	cd $(BUILD_DIR) && ./$(BINARY_NAME) -config ../$(CONFIG_FILE) -log 2