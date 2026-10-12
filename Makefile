BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X 'main.BuildTime=$(BUILD_TIME)'
BINDIR ?= .

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/quickFFmpeg ./cmd/quickffmpeg

build-worker:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/quickffmpeg-linux-amd64 ./cmd/quickffmpeg

run: BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
run: build
	@true
