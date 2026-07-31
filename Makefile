APP_NAME := sla-agent
BUILD_DIR := bin
MAIN_PKG := ./cmd/sla-agent

.PHONY: all build clean run

all: build

build:
	@echo "Building static binary for VyOS (linux/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"
	@ls -lh $(BUILD_DIR)/$(APP_NAME)

clean:
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)

run: build
	@echo "Running $(APP_NAME)... (Requires root/sudo for raw sockets)"
	sudo ./$(BUILD_DIR)/$(APP_NAME)
