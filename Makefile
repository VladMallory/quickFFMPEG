BINDIR ?= .

build:
	go build -o $(BINDIR)/quickFFmpeg

run: build
	@true
