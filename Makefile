.PHONY: vet test build lint check

# Pinned so a new golangci-lint release cannot turn CI red by itself; bump it
# deliberately. Run through the Go module proxy, which the runners already use,
# so no binary is downloaded from GitHub releases.
GOLANGCI_LINT_VERSION := v2.14.0

vet:
	go vet ./...

test:
	go test -race ./...

build:
	CGO_ENABLED=0 go build ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

# What CI runs, and what to run before pushing.
check: vet test build lint
