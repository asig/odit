# Simple Makefile for Oberon Disk Image Tool (Go)

BINARY=odit
SRC=odit.go

.PHONY: all build clean test

all: build

build:
	go build -o $(BINARY) $(SRC)

test:
	go test ./...

clean:
	rm -f $(BINARY)

fmt:
	go fmt ./...

lint:
	golint ./...
