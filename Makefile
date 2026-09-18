# Robot Dreams build entry points. `make ci` is exactly what CI runs
# (.github/workflows/ci.yml), so a green `make ci` locally means a green PR.

.PHONY: build test test-race test-integration fmt vet lint vulncheck ci

GOLANGCI    ?= golangci-lint
GOVULNCHECK ?= govulncheck
VERSION     ?= 0.1.0-dev

build:
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/dream ./cmd/dream

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	go test -tags=integration ./test/...

# gofmt, then goimports/gofumpt via golangci-lint's formatters.
fmt:
	gofmt -w .
	$(GOLANGCI) fmt ./...

vet:
	go vet ./...

lint:
	$(GOLANGCI) run ./...

vulncheck:
	$(GOVULNCHECK) ./...

# Everything CI runs, in CI's order.
ci: vet lint test-race vulncheck
	go build ./...
	node npm/scripts/install.js --self-test
