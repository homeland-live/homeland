GO_META_MOD := github.com/avakarev/go-util/buildmeta

LDFLAGS += -s -w
LDFLAGS += -X ${GO_META_MOD}.Commit=${GITHUB_SHA}
LDFLAGS += -X ${GO_META_MOD}.Ref=${GITHUB_REF}
LDFLAGS += -X ${GO_META_MOD}.BuildTimeUTC=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

ifdef CGO_ENABLED
LDFLAGS += -linkmode external
LDFLAGS += -extldflags "-static"
export CGO_CFLAGS = "-D_LARGEFILE64_SOURCE" # https://github.com/mattn/go-sqlite3/issues/1164
endif

ifneq (,$(wildcard .env))
include .env
endif

lint-install:
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

lint:
	@echo ">> Running golangci-lint..."
	@golangci-lint version
	@golangci-lint run ./...

vet:
	@echo ">> Vetting..."
	@go vet ./...

test-install:
	@go install gotest.tools/gotestsum@v1.13.0

test:
	@echo ">> Running tests..."
	@gotestsum --format standard-verbose -- -v -race ./...
.PHONY: test

ci: lint vet test

build:
	@echo ">> Building ./bin/server..."
	@go build -a -ldflags '${LDFLAGS}' -o ./bin/server ./cmd/server

server-run: build
	@while [ ! -f ./ui/dist/index.html ]; do sleep 0.2; done
	@./bin/server

ui-clean:
	@echo "deleting ./ui/dist"
	@rm -rf ./ui/dist

ui-run:
	@npm install --prefix ./ui
	@npm run --prefix ./ui watch

dev: ui-clean
	@go install github.com/mattn/goreman@v0.3.19
	@echo "running Procfile by goreman $$(goreman version)"
	@goreman -exit-on-error=true -rpc-server=false -set-ports=false start
