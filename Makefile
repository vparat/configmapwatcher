default: build
GO_VERSION = $(shell curl -s https://go.dev/dl/?mode=json | jq -r '.[0].version' | sed 's/go//')
FUZZTIME ?= 30s
FUZZ_PARALLEL ?= 2

.PHONY: default clean update-packages fix lint lint-strict test test-clean test-cover test-fuzz benchmarks build race tidy

clean:
	go clean ./...
	rm -f cover.out

update-packages:
	go mod edit -go=${GO_VERSION}
	go get -u ./...
	go mod tidy

fix:
	go fix ./...

lint:
	golangci-lint run

lint-strict:
	gocritic check -enableAll ./...

test:
	go test ./...

test-clean:
	go clean -testcache
	go test ./...

test-cover:
	go test ./... -coverprofile cover.out
	go tool cover -html=cover.out

test-fuzz:
	go test -run='^$$' -fuzz='^FuzzWatchHandlers$$' -fuzztime=$(FUZZTIME) -parallel=$(FUZZ_PARALLEL) .
	go test -run='^$$' -fuzz='^FuzzWatchAsBytes$$' -fuzztime=$(FUZZTIME) -parallel=$(FUZZ_PARALLEL) .

benchmarks:
	go test -benchmem -bench=. ./... -tags=benchmark

build:
	go build ./...

race:
	go test -race ./...

tidy:
	go mod tidy

