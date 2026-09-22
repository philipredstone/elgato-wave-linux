PREFIX  ?= $(HOME)/.local
UNITDIR ?= $(HOME)/.config/systemd/user
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

GOFLAGS := -trimpath -ldflags '-s -w -X main.version=$(VERSION)'

.PHONY: build test lint install install-udev enable uninstall clean

build:
	go build $(GOFLAGS) -o waved ./cmd/waved

test:
	go test ./...

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

install: build
	install -Dm755 waved $(PREFIX)/bin/waved
	install -Dm644 systemd/waved.service $(UNITDIR)/waved.service
	-systemctl --user daemon-reload

install-udev:
	install -Dm644 udev/60-waved.rules /etc/udev/rules.d/60-waved.rules
	udevadm control --reload
	udevadm trigger --subsystem-match=usb

enable:
	systemctl --user enable --now waved.service

uninstall:
	-systemctl --user disable --now waved.service
	rm -f $(PREFIX)/bin/waved $(UNITDIR)/waved.service

clean:
	rm -f waved
