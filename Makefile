## Variables for UPX
UPX_VERSION  := 5.0.2
UPX_ARCHIVE  := upx-$(UPX_VERSION)-amd64_linux.tar.xz
UPX_DIR      := upx-$(UPX_VERSION)-amd64_linux
UPX_BIN      := /usr/local/bin/upx
UPX_URL      := https://github.com/upx/upx/releases/download/v$(UPX_VERSION)/$(UPX_ARCHIVE)

## Variables for TailwindCSS
TAILWIND     := /usr/local/bin/tailwindcss
CSS_IN       := web/tailwindcss/input.css
CSS_OUT      := web/static/css/style.css

## Variables for Go application
APP        := zabbix-statuspage
BIN        := bin/$(APP)
PREFIX     := zabbix-statuspage/cmd
VERSION    := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS    := -trimpath -ldflags "-s -w \
	-X $(PREFIX).Version=$(VERSION) \
	-X $(PREFIX).BuildDate=$(BUILD_TIME) \
	-X $(PREFIX).GitCommit=$(GIT_COMMIT)"

.PHONY: all build build-prod css run test lint clean tidy deps install-upx help

all: clean css build

css:
	@echo "Compiling CSS..."
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

build: css
	@echo "Building $(APP)..."
	CGO_ENABLED=0 go build -o $(BIN) $(LDFLAGS) .
	upx --best --lzma $(BIN)

build-prod:
	@echo "Building $(APP) (production + UPX)..."
	CGO_ENABLED=0 go build -o $(BIN) $(LDFLAGS) .
	upx --best --lzma $(BIN)

run:
	@echo "Starting $(APP)..."
	./$(BIN) serve

test:
	@echo "Running tests..."
	go test -race ./...

lint:
	@echo "Running linter..."
	golangci-lint run

clean:
	@echo "Cleaning up..."
	rm -rf bin/ dist/

tidy:
	@echo "Tidying go modules..."
	go mod tidy

deps:
	@echo "Installing dependencies..."
	go mod download

install-upx:
	@echo "Installing UPX $(UPX_VERSION)..."
	curl -ksSL "$(UPX_URL)" -o "$(UPX_ARCHIVE)"
	tar -xf "$(UPX_ARCHIVE)"
	chmod +x "$(UPX_DIR)/upx"
	mv "$(UPX_DIR)/upx" "$(UPX_BIN)"
	rm -rf "$(UPX_DIR)" "$(UPX_ARCHIVE)"

docker:
	@echo "Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(APP):$(VERSION) .

release:
	@echo "Building release binary (linux/amd64)..."
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(APP)-linux-amd64 .
	upx --best --lzma dist/$(APP)-linux-amd64

help:
	@echo "Makefile commands:"
	@echo "  css          - Compile TailwindCSS (input.css → style.css)"
	@echo "  build        - Compile CSS + build binary"
	@echo "  build-prod   - Compile CSS + build binary + UPX compression"
	@echo "  run          - Build and start server"
	@echo "  test         - Run tests with race detector"
	@echo "  lint         - Run golangci-lint"
	@echo "  clean        - Remove bin/ and dist/"
	@echo "  tidy         - go mod tidy"
	@echo "  deps         - go mod download"
	@echo "  docker       - Build Docker image"
	@echo "  release      - Compile linux/amd64 with UPX"
	@echo "  install-upx  - Download and install UPX $(UPX_VERSION)"
