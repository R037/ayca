VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX  ?= $(HOME)/.local

.PHONY: build install test lint clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/ayca ./cmd/ayca

# Installs to $(PREFIX)/bin (default ~/.local/bin). Deliberately not
# `go install`: version managers such as mise point GOBIN into the
# toolchain's own directory, which makes the binary unavailable elsewhere.
install: build
	install -d "$(PREFIX)/bin"
	install -m 0755 bin/ayca "$(PREFIX)/bin/ayca"
	@echo "installed $(PREFIX)/bin/ayca"

test:
	go test ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

clean:
	rm -rf bin
