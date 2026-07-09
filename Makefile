BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X 'main.BuildTime=$(BUILD_TIME)'
BINDIR ?= .

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/quickFFmpeg

run: BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
run: build
	@echo "Built at $(BUILD_TIME)"
