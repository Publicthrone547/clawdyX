BIN   := bin/clawdyx
GOBIN := $(shell go env GOPATH)/bin
PKG   := ./...

.PHONY: identities notices help setup run start stop restart status logs doctor build test test-integration lint fmt check-secrets check install clean

help: ## list the everyday commands
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

## Everyday commands
setup: build ## first-time setup: bot token, your Telegram ID, project folder (writes private config)
	$(BIN) setup

run: build ## run the bot in the foreground; Ctrl-C stops it
	$(BIN) run

start: install ## run the bot in the background now and at every login (LaunchAgent)
	$(GOBIN)/clawdyx service start

stop: ## stop the background bot (it stays off until make start)
	@test -x $(GOBIN)/clawdyx || (echo "not installed yet; run make start"; exit 1)
	$(GOBIN)/clawdyx service stop

restart: install ## rebuild, reinstall and restart the background bot
	$(GOBIN)/clawdyx service restart

status: ## is the bot running?
	@test -x $(GOBIN)/clawdyx || (echo "not installed yet; run make start"; exit 1)
	$(GOBIN)/clawdyx service status

logs: ## follow the bot log
	tail -n 50 -f "$(HOME)/Library/Logs/clawdyx/bot.log"

doctor: build ## read-only checks: config, token, Claude CLI, auth, Telegram
	$(BIN) check

## Development
build:
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/clawdyx

test:
	go test -race $(PKG)

test-integration:
	go test -race -tags integration $(PKG)

fmt:
	gofmt -w .

lint:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet $(PKG)
	@command -v staticcheck >/dev/null && staticcheck $(PKG) || echo "staticcheck not installed; skipped"
	@command -v govulncheck >/dev/null && govulncheck $(PKG) || echo "govulncheck not installed; skipped"

# Fails if anything that looks like a Telegram bot token or Anthropic key is tracked or staged.
check-secrets:
	@if git grep -nIE '[0-9]{8,10}:[A-Za-z0-9_-]{35}|sk-ant-[A-Za-z0-9_-]{10,}' -- . ':!Makefile' ':!docs/*' ':!internal/observability/*' ; then \
		echo "possible secret found"; exit 1; else echo "no secrets found"; fi

check: lint test check-secrets

notices: ## regenerate THIRD_PARTY_NOTICES.md
	scripts/gen-notices.sh

# CLAWDYX_SIGN_IDENTITY="Apple Development: …" (or a self-signed code-signing
# certificate's name) gives the binary a stable identity, so macOS keeps the
# Screen Recording and Accessibility grants across rebuilds.
install: build
	install -m 0755 $(BIN) $(GOBIN)/clawdyx
	@if [ -n "$$CLAWDYX_SIGN_IDENTITY" ]; then \
		codesign --force --identifier io.github.publicthrone547.clawdyx --sign "$$CLAWDYX_SIGN_IDENTITY" $(GOBIN)/clawdyx && echo "signed with $$CLAWDYX_SIGN_IDENTITY"; \
	else \
		echo "note: unsigned build; macOS Screen Recording and Accessibility grants for clawdyx reset after each rebuild (set CLAWDYX_SIGN_IDENTITY to keep them)"; \
	fi

identities: ## list code-signing identities usable for CLAWDYX_SIGN_IDENTITY
	@security find-identity -v -p codesigning

clean:
	rm -rf bin coverage.out
