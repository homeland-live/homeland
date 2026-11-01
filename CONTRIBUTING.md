# Contributing

Thanks for helping with Homeland. Issues and pull requests are welcome; for a larger change, open an issue first so
we can agree on the approach. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains how the pieces fit together.

## Setup

You need:

- Go 1.27+ with CGO and a C compiler (the SQLite driver is `mattn/go-sqlite3`).
- Node 24 for the UI and the docs example checks.
- An MQTT broker with Zigbee2MQTT for running it for real. The example stack in [`docs/compose`](docs/compose)
  works; the tests need neither.

```bash
cp .env.example .env    # the Makefile loads .env; set HL_Z2M_MQTT_BROKER at least
make lint-install test-install
make dev                # server + UI watcher (long-running)
```

## Checks

Run these before opening a PR; CI runs the same:

```bash
make ci                 # golangci-lint (with gofmt/goimports), go vet, go test -race, docs-check
make fmt                # fix what the formatters report
cd ui && npm run ci     # UI lint, type-check and tests
```

The make targets cover `./cmd/...` and `./internal/...`. Plain `./...` also matches a Go package under
`ui/node_modules`, so name those paths when you run `go test` yourself.

## Conventions

- **Layering.** Keep `internal/mqtt` free of Zigbee2MQTT logic, keep `internal/web` HTTP-only, and let only
  `web/context` construct and close dependencies.
- **Script-author mistakes are never fatal.** Report them as a broken handler or a log line, not a startup error.
- **Comments** document what a symbol guarantees (ordering, concurrency, nil behavior), briefly, starting with its
  name. Leave history to the commit log.
- **Logs and errors** start with the package path and use slog key/values:
  `slog.Error("svc/event: script error", "device", name, "err", err)`,
  `fmt.Errorf("svc/scripts: compile %q: %w", name, err)`.
- **Tests** live in a separate `_test` package (use `export_test.go` for internals), use testify (`require` for
  preconditions, `assert` for checks), use fakes instead of real brokers, and pass with `-race`. Prefer
  `assert.Eventually`/`Never` over sleeps. Shared helpers: `github.com/avakarev/go-util/testutil/slog` captures logs and
  `internal/mqtt/mqtttest` provides a broker-less MQTT client to build a z2m client over and a loopback broker for
  a real paho client.
- **Migrations** in `internal/db/migration.go` are append-only: never edit one that has shipped.
- **Docs.** When behavior changes, update the README, [`docs/script.d.ts`](docs/script.d.ts) (the script author
  contract), and [`.env.example`](.env.example) for configuration. The example gallery in
  [`docs/README.md`](docs/README.md) is generated from [`docs/examples`](docs/examples): edit those files and run
  `make docs`, never the generated code blocks. `make docs-examples-fmt` formats the examples,
  `make docs-examples-fmt-check` checks their formatting and `make docs-examples-ts` type-checks them, using the
  tools in `ui/node_modules`; the docs workflow runs both checks. Release notes are written in the GitHub release, so
  describe user-visible changes in the PR description.
- **New endpoints.** The API has no authentication; say so in the PR when an endpoint exposes scripts, state or
  history.

## Commits and PRs

Write commit messages and PR titles following [Conventional Commits](https://www.conventionalcommits.org/), scoped
by area, e.g. `fix(server): ...` or `chore(ui): ...`. In the PR description, summarize the change and say how you
tested it.
