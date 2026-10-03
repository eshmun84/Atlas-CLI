.PHONY: build test fmt vet check clean

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

clean:
	rm -rf bin/
