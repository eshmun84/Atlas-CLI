.PHONY: build install test fmt vet check clean smoke-mvp

BINARY := bin/atlas
PREFIX ?= $(HOME)/.local
VERSION ?=

# Same ldflags for the published binary and for installlocal, so Home's
# atlas_version matches the binary. Empty when VERSION is unset (source default).
VERSION_LDFLAGS :=
ifneq ($(VERSION),)
VERSION_LDFLAGS := -ldflags "-X github.com/eshmun84/Atlas-CLI/internal/version.Version=$(VERSION)"
endif

build:
	go build $(VERSION_LDFLAGS) -o $(BINARY) ./cmd/atlas

# Local install for Alpha validation. Defaults to ~/.local/bin when that tree exists
# in the environment; smoke uses a temporary PREFIX and never publishes.
# Stages the binary, materializes Atlas Home (ATLAS_HOME or ~/.atlas), then
# renames into place. A failure before publish leaves the previous binary in place
# and restores the Home mirror snapshot when this install mutated it.
install: build
	go run $(VERSION_LDFLAGS) ./scripts/installlocal -binary="$(BINARY)" -prefix="$(PREFIX)"

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
