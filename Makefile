.PHONY: build test fmt vet check clean smoke-mvp

BINARY := bin/atlas

build:
	go build -o $(BINARY) ./cmd/atlas

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
