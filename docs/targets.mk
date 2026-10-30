# Docs targets, included by the root Makefile. Recipes run from the repo root.
#
# The docs-examples-* targets use the tools in ui/node_modules (oxfmt and
# typescript, versioned by ui/package-lock.json), installing them on first use.

UI_BIN := ui/node_modules/.bin

docs:
	@echo ">> Generating docs..."
	@go run ./cmd/gen-docs
.PHONY: docs # docs/ exists, so without this make thinks the target is up to date

docs-check:
	@echo ">> Checking docs are up to date..."
	@go run ./cmd/gen-docs -check
.PHONY: docs-check

# npm writes this file on every install; an order-only prerequisite, so it only
# installs when ui/node_modules is missing.
ui/node_modules/.package-lock.json:
	@echo ">> Installing UI dependencies..."
	@npm ci --prefix ui

EXAMPLE_SCRIPTS := docs/examples docs/compose/homeland/scripts
# The examples share the UI's formatting style.
OXFMT_CONFIG := ui/.oxfmtrc.json

# Formats the example and compose scripts, then regenerates docs/README.md
# from them.
docs-examples-fmt: | ui/node_modules/.package-lock.json
	@echo ">> Formatting example scripts..."
	@$(UI_BIN)/oxfmt -c $(OXFMT_CONFIG) $(EXAMPLE_SCRIPTS)
	@$(MAKE) --no-print-directory docs
.PHONY: docs-examples-fmt

# Fails if the example or compose scripts aren't formatted; writes nothing.
docs-examples-fmt-check: | ui/node_modules/.package-lock.json
	@echo ">> Checking example scripts are formatted..."
	@$(UI_BIN)/oxfmt -c $(OXFMT_CONFIG) --check $(EXAMPLE_SCRIPTS)
.PHONY: docs-examples-fmt-check

# Type-checks the example and compose scripts against script.d.ts.
docs-examples-ts: | ui/node_modules/.package-lock.json
	@echo ">> Type-checking example scripts..."
	@$(UI_BIN)/tsc -p docs/tsconfig.script.json
.PHONY: docs-examples-ts
