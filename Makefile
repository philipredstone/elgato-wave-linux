PREFIX  ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

GOFLAGS := -trimpath -ldflags '-s -w -X main.version=$(VERSION)'

.PHONY: build test lint install clean

build:
	go build $(GOFLAGS) -o waved ./cmd/waved

test:
	go test ./...

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

install: build
	install -Dm755 waved $(PREFIX)/bin/waved
	$(PREFIX)/bin/waved install

clean:
	rm -f waved
