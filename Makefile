.PHONY: all build build-macos build-windows run clean test

# Application name
APP_NAME=BuchISY
BINARY_NAME=buchisy

# Build directory
BUILD_DIR=build

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Main package path
MAIN_PATH=./cmd/buchisy

# Default target
all: test build

# Build for current platform
build:
	@echo "Building for current platform..."
	@mkdir -p $(BUILD_DIR)
	MACOSX_DEPLOYMENT_TARGET=15.0 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for macOS (current architecture only, CGO required)
build-macos:
	@echo "Building for macOS (current architecture)..."
	@mkdir -p $(BUILD_DIR)
	MACOSX_DEPLOYMENT_TARGET=15.0 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-macos $(MAIN_PATH)
	@echo "macOS build complete: $(BUILD_DIR)/$(BINARY_NAME)-macos"

# Build for Windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Windows build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

# Package for macOS (creates .app bundle)
package-macos:
	@echo "Packaging for macOS..."
	@test -f ~/go/bin/fyne || (echo "Installing fyne command..." && go install fyne.io/fyne/v2/cmd/fyne@latest)
	MACOSX_DEPLOYMENT_TARGET=15.0 ~/go/bin/fyne package -os darwin -name $(APP_NAME) -src $(MAIN_PATH)
	@echo "Copying assets into app bundle..."
	@mkdir -p $(APP_NAME).app/Contents/Resources
	@cp -r assets $(APP_NAME).app/Contents/Resources/
	@echo "macOS package created: $(APP_NAME).app"
	@echo "You can now run: open $(APP_NAME).app"

# Package for Windows (creates .exe with icon) - must be run on Windows
package-windows:
	@echo "Packaging for Windows..."
	@which fyne > /dev/null || (echo "Installing fyne command..." && go install fyne.io/fyne/v2/cmd/fyne@latest)
	fyne package -os windows -name $(APP_NAME) -src $(MAIN_PATH)
	@echo "Windows package created: $(APP_NAME).exe"

# Run the application
run:
	@echo "Running $(APP_NAME)..."
	$(GOCMD) run $(MAIN_PATH)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f $(APP_NAME).app $(APP_NAME).exe
	rm -f coverage.out coverage.html
	@echo "Clean complete"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies updated"

# Install fyne command (for packaging)
install-fyne:
	@echo "Installing fyne command..."
	$(GOCMD) install fyne.io/fyne/v2/cmd/fyne@latest
	@echo "fyne command installed"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
	@echo "Format complete"

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	@which golangci-lint > /dev/null || (echo "Error: golangci-lint not found. Install from: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run
	@echo "Lint complete"

# Help
help:
	@echo "BuchISY - Makefile commands:"
	@echo ""
	@echo "  make build              - Build for current platform"
	@echo "  make build-macos        - Build for macOS (Intel + ARM)"
	@echo "  make build-windows      - Build for Windows"
	@echo "  make package-macos      - Create macOS .app bundle"
	@echo "  make package-windows    - Create Windows .exe"
	@echo "  make run                - Run the application"
	@echo "  make test               - Run tests"
	@echo "  make test-coverage      - Run tests with coverage report"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make deps               - Download and tidy dependencies"
	@echo "  make fmt                - Format code"
	@echo "  make lint               - Lint code (requires golangci-lint)"
	@echo "  make help               - Show this help message"
	@echo ""
