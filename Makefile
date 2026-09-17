# Robot Dreams build entry points. `make ci` is exactly what CI runs
# (.github/workflows/ci.yml), so a green `make ci` locally means a green PR.

.PHONY: build go-build test go-test test-race test-integration fmt vet lint \
        lint-go lint-ui generate ci ui-install ui-build ui-clean ui-dev \
        ui-check ui-test vulncheck

UI_DIR      := internal/dashboard/ui
WEB_DIR     := internal/dashboard/web
GOLANGCI    ?= golangci-lint
GOVULNCHECK ?= govulncheck
VERSION     ?= 0.1.0-dev

# The dream binary, with Mission Control embedded. Depends on the UI build
# because the dashboard's Vite output is not committed (see
# internal/dashboard/embed.go): without it the binary ships the placeholder
# page. Needs Node 22+; `make go-build` is the Node-free variant.
build: ui-build go-build

# Compile only the Go side, embedding whatever is in $(WEB_DIR) right now
# (the committed placeholder on a fresh clone, or the last UI build).
go-build:
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/dream ./cmd/dream

# --- Mission Control (internal/dashboard/ui, an npm workspace) -----------

# `npm ci` only when the lockfile is newer than the installed tree, so
# repeated `make build` runs do not reinstall. Touching the stamp is what
# npm ci does itself (it writes node_modules/.package-lock.json).
$(UI_DIR)/node_modules/.package-lock.json: $(UI_DIR)/package-lock.json
	cd $(UI_DIR) && npm ci

ui-install: $(UI_DIR)/node_modules/.package-lock.json

# Build the dashboard into $(WEB_DIR)/dist (typecheck + Vite). dist/ is
# gitignored; the committed $(WEB_DIR)/placeholder page is what the binary
# serves when dist/ is absent, so a build never touches a tracked file.
ui-build: ui-install
	cd $(UI_DIR) && npm run build

# Drop the dashboard build so the next go-build embeds the placeholder.
ui-clean:
	rm -rf $(WEB_DIR)/dist

# Dashboard dev loop: run `go run ./cmd/dream simulate --ui-dev` (does
# everything), or start any server and point the Vite proxy at it:
#   DREAM_API=http://127.0.0.1:<port> make ui-dev
ui-dev: ui-install
	cd $(UI_DIR) && npm run dev

# The UI's own quality gates, as CI runs them: eslint, prettier (check
# only), tsc, vitest, then the library build + the smoke consumer that
# installs the built package through its exports map.
ui-check: ui-install
	cd $(UI_DIR) && npm run lint && npm run format:check && npm run typecheck && npm run test && npm run build:lib && npm run smoke

ui-test: ui-install
	cd $(UI_DIR) && npm run test

# --- Go ---------------------------------------------------------------

# `go generate ./...` runs the //go:generate in internal/dashboard/embed.go,
# i.e. `make ui-build`.
generate:
	go generate ./...

# Unit tests on both sides: Go and the UI's vitest suite. `go-test` is
# the Node-free half.
test: go-test ui-test

go-test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	go test -tags=integration ./test/...

# gofmt + goimports/gofumpt via golangci-lint's formatters, then prettier
# for the UI.
fmt: ui-install
	gofmt -w .
	$(GOLANGCI) fmt ./...
	cd $(UI_DIR) && npm run format

vet:
	go vet ./...

lint-go:
	$(GOLANGCI) run ./...

lint-ui: ui-install
	cd $(UI_DIR) && npm run lint && npm run format:check

lint: lint-go lint-ui

vulncheck:
	$(GOVULNCHECK) ./...

# Everything CI runs, in CI's order. The Go tests run AFTER the UI build so
# internal/dashboard's asset tests see the real bundle, as in CI.
ci: ui-check ui-build vet lint-go test-race vulncheck
	go build ./...
	node npm/scripts/install.js --self-test
