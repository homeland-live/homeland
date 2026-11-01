GO_META_MOD := github.com/avakarev/go-util/buildmeta

# The Go packages to lint, vet and test. Not ./...: that also matches a Go
# package under ui/node_modules.
GO_PKGS := ./cmd/... ./internal/...

LDFLAGS += -s -w
LDFLAGS += -X ${GO_META_MOD}.Commit=${GITHUB_SHA}
LDFLAGS += -X ${GO_META_MOD}.Ref=${GITHUB_REF}
LDFLAGS += -X ${GO_META_MOD}.BuildTimeUTC=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# STATIC=1 links a fully static binary, as the scratch image needs. It needs
# CGO and a static libc such as Alpine's musl, so it doesn't work on macOS.
ifeq ($(STATIC),1)
export CGO_ENABLED := 1
LDFLAGS += -linkmode external
LDFLAGS += -extldflags "-static"
# https://github.com/mattn/go-sqlite3/issues/1164
export CGO_CFLAGS := -D_LARGEFILE64_SOURCE
endif

ifneq (,$(wildcard .env))
include .env
endif

lint-install:
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

lint:
	@echo ">> Running golangci-lint..."
	@golangci-lint version
	@golangci-lint run $(GO_PKGS)

# Applies the formatters lint enforces (gofmt, goimports).
fmt:
	@echo ">> Formatting Go code..."
	@golangci-lint fmt $(GO_PKGS)
.PHONY: fmt

vet:
	@echo ">> Vetting..."
	@go vet $(GO_PKGS)

test-install:
	@go install gotest.tools/gotestsum@v1.13.0

test:
	@echo ">> Running tests..."
	@gotestsum --format standard-verbose -- -v -race $(GO_PKGS)
.PHONY: test

# Installs the UI dependencies when ui/node_modules is missing or older than
# the lockfile. npm writes .package-lock.json on every install.
ui/node_modules/.package-lock.json: ui/package-lock.json
	@echo ">> Installing UI dependencies..."
	@npm ci --prefix ui

# Optional: the Docker builder runs `make build` without docs/.
-include docs/targets.mk

ci: lint vet test docs-check

build:
	@echo ">> Building ./bin/server..."
	@go build -ldflags '${LDFLAGS}' -o ./bin/server ./cmd/server

server-run: build
	@while [ ! -f ./ui/dist/index.html ]; do sleep 0.2; done
	@./bin/server

ui-clean:
	@echo "deleting ./ui/dist"
	@rm -rf ./ui/dist

ui-run: ui/node_modules/.package-lock.json
	@npm run --prefix ./ui watch

dev: ui-clean
	@go install github.com/mattn/goreman@v0.3.19
	@echo "running Procfile by goreman $$(goreman version)"
	@goreman -exit-on-error=true -rpc-server=false -set-ports=false start
