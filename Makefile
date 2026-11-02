.PHONY: vet test build check

vet:
	go vet ./...

test:
	go test -race ./...

build:
	CGO_ENABLED=0 go build ./...

# What CI runs, and what to run before pushing.
check: vet test build
