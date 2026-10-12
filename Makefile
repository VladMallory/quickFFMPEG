BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X 'main.BuildTime=$(BUILD_TIME)'
BINDIR ?= .

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/quickFFmpeg ./cmd/quickffmpeg

run: BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
run: build
	@true
