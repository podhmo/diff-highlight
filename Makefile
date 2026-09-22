.PHONY: build test lint ci

build:
	go build -o diff-highlight .

test:
	go test -count=1 ./...

lint:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	go vet ./...
	go fix ./...

ci: lint test
