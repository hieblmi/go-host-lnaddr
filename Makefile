# Variables (can be overridden from the command line)
GO ?= go
BINARY_NAME ?= gohostlnaddr
PKG := ./...
MAIN_PKG := .
CGO_ENABLED ?= 0
GOFLAGS ?=
LDFLAGS ?=
# Try to derive version from git; ignore errors
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILDFLAGS := -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" $(GOFLAGS)

# Install-time variables (require GOBIN)
GOBIN_PATH := $(shell $(GO) env GOBIN)
ifeq ($(strip $(GOBIN_PATH)),)
  $(error GOBIN is not set (go env GOBIN returned empty). Please set GOBIN to your Go bin directory)
endif
INSTALL_DIR := $(GOBIN_PATH)
INSTALL_NAME ?= lnaddr

# Default target
.PHONY: make
make:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(BUILDFLAGS) -o $(BINARY_NAME) $(MAIN_PKG)

# Install target installs the binary named "lnaddr" into GOBIN (or GOPATH/bin)
.PHONY: install
install:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(BUILDFLAGS) -o $(INSTALL_NAME) $(MAIN_PKG)
	install -d $(INSTALL_DIR)
	install -m 0755 $(INSTALL_NAME) $(INSTALL_DIR)/$(INSTALL_NAME)

# Test target runs unit tests across all packages; include race detector when supported
.PHONY: test
test:
	$(GO) test -race -count=1 $(GOFLAGS) $(PKG)

# Clean up built artifacts
.PHONY: clean
clean:
	rm -f $(BINARY_NAME) $(INSTALL_NAME)

# Help message
.PHONY: help
help:
	@echo "Targets:"
	@echo "  make      - Build the project binary (BINARY_NAME=$(BINARY_NAME))"
	@echo "  install   - Install the lnaddr binary to \$$GOBIN (GOBIN must be set)"
	@echo "  test      - Run all tests with race detector"
	@echo "  clean     - Remove built binary"
	@echo "Variables:"
	@echo "  BINARY_NAME (default: $(BINARY_NAME))"
	@echo "  INSTALL_NAME (default: $(INSTALL_NAME))"
	@echo "  CGO_ENABLED (default: $(CGO_ENABLED))"
	@echo "  GOFLAGS     (default: '$(GOFLAGS)')"
	@echo "  LDFLAGS     (default: '$(LDFLAGS)')"