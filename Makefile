.PHONY: build install test fmt vet check clean smoke-mvp

BINARY := bin/atlas
PREFIX ?= $(HOME)/.local
VERSION ?=

build:
	@if [ -n "$(VERSION)" ]; then \
		go build -ldflags "-X github.com/eshmun84/Atlas-CLI/internal/version.Version=$(VERSION)" -o $(BINARY) ./cmd/atlas; \
	else \
		go build -o $(BINARY) ./cmd/atlas; \
	fi

# Local install for Alpha validation. Defaults to ~/.local/bin when that tree exists
# in the environment; smoke uses a temporary PREFIX and never publishes.
install: build
	mkdir -p "$(PREFIX)/bin"
	cp "$(BINARY)" "$(PREFIX)/bin/atlas"
	@echo "installed $(PREFIX)/bin/atlas"

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test ./...

smoke-mvp:
	@bash scripts/smoke-mvp.sh

clean:
	rm -rf bin/
