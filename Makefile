# Memcore build and release targets. Override VERSION on the command line for a
# tagged build: make image VERSION=1.2.3

IMAGE   ?= memcore/memcored
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
PLATFORMS ?= linux/amd64,linux/arm64

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build test race vet lint tidy bench clean image push

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/memcored ./cmd/memcored

test:
	go test ./...

race:
	CGO_ENABLED=1 go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

bench:
	go test -run '^$$' -bench . -benchmem ./internal/command/

clean:
	rm -rf bin dist

# Multi-arch image build. Requires a buildx builder; tags the immutable version
# and latest. latest is intended to be published only from the default branch.
image:
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--tag $(IMAGE):$(VERSION) \
		--tag $(IMAGE):latest \
		.

push:
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--tag $(IMAGE):$(VERSION) \
		--tag $(IMAGE):latest \
		--push \
		.
