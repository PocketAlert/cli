VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint snapshot install

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o pocketalert ./cmd/pocketalert

test:
	go test -race ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

snapshot:
	goreleaser release --snapshot --clean

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/pocketalert
