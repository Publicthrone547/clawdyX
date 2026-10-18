BIN     := bin/clawdyx
GOBIN   := $(shell go env GOPATH)/bin
PKG     := ./...
VERSION := 0.3.3
MACENV  := CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=14.0 CGO_CFLAGS=-mmacosx-version-min=14.0 CGO_LDFLAGS=-mmacosx-version-min=14.0
WINENV  := GOOS=windows GOARCH=amd64 CGO_ENABLED=0
WINDIR  := bin/windows/clawdyX
REL     := bin/release-$(VERSION)

.PHONY: check-all app dmg build-windows vet-windows release helper frontend bindings dev test-desktop identities notices help setup run start stop restart status logs doctor build test test-integration lint fmt check-secrets check install clean

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
	CGO_ENABLED=0 go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(BIN) ./cmd/clawdyx

## Desktop app
app: build helper frontend ## build bin/clawdyX.app (desktop app + worker + capture helper)
	cd desktop && $(MACENV) go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -X main.version=$(VERSION)" -o ../bin/clawdyX-gui .
	VERSION=$(VERSION) scripts/bundle.sh

dmg: app ## build bin/clawdyX-$(VERSION).dmg for installing by drag and drop
	rm -rf bin/dmg && mkdir -p bin/dmg && cp -R bin/clawdyX.app bin/dmg/ && ln -s /Applications bin/dmg/Applications
	hdiutil create -volname clawdyX -srcfolder bin/dmg -ov -format UDZO bin/clawdyX-$(VERSION).dmg >/dev/null
	rm -rf bin/dmg && ls -lh bin/clawdyX-$(VERSION).dmg

## Windows (cross-compiled from this Mac; no CGO, no extra toolchain)
build-windows: frontend ## build bin/windows/clawdyX/ for Windows amd64: clawdyX.exe (app) + helpers/clawdyx.exe (worker)
	rm -rf bin/windows && mkdir -p $(WINDIR)/helpers
	$(WINENV) go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(WINDIR)/helpers/clawdyx.exe ./cmd/clawdyx
	@if command -v wails3 >/dev/null; then \
		sed 's/__VERSION__/$(VERSION)/g' desktop/build/windows/info.json > bin/windows/info.json && \
		wails3 generate syso -arch amd64 -icon desktop/build/windows/icon.ico -manifest desktop/build/windows/clawdyX.exe.manifest \
			-info bin/windows/info.json -out desktop/rsrc_windows_amd64.syso >/dev/null; \
	else echo "note: wails3 not installed; clawdyX.exe gets no icon or manifest (see make bindings)"; fi
	cd desktop && $(WINENV) go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui -X main.version=$(VERSION)" -o ../$(WINDIR)/clawdyX.exe .
	rm -f desktop/rsrc_windows_amd64.syso bin/windows/info.json
	cp LICENSE THIRD_PARTY_NOTICES.md README.md $(WINDIR)/
	@ls -lh $(WINDIR)/clawdyX.exe $(WINDIR)/helpers/clawdyx.exe

vet-windows: ## type-check everything (including tests) for Windows
	$(WINENV) go vet $(PKG)
	cd desktop && $(WINENV) go vet ./...

## Release
release: check-all dmg build-windows ## build every release file into bin/release-$(VERSION) with SHA256SUMS
	rm -rf $(REL) bin/tar && mkdir -p $(REL) bin/tar/clawdyx-$(VERSION)-darwin-arm64
	cp bin/clawdyx LICENSE THIRD_PARTY_NOTICES.md README.md bin/tar/clawdyx-$(VERSION)-darwin-arm64/
	tar -C bin/tar -czf $(REL)/clawdyx-$(VERSION)-darwin-arm64.tar.gz clawdyx-$(VERSION)-darwin-arm64
	cp bin/clawdyX-$(VERSION).dmg $(REL)/
	cd bin/windows && rm -f ../../$(REL)/clawdyX-$(VERSION)-windows-amd64.zip && zip -qr -X ../../$(REL)/clawdyX-$(VERSION)-windows-amd64.zip clawdyX
	rm -rf bin/tar
	cd $(REL) && shasum -a 256 *.dmg *.tar.gz *.zip > SHA256SUMS && cat SHA256SUMS

helper:
	swiftc -O -parse-as-library -target $$(uname -m)-apple-macos14.0 desktop/helper/main.swift -o bin/clawdyx-screenshot

frontend:
	cd desktop/frontend && (test -d node_modules || npm ci) && npm run build

bindings: ## regenerate the TypeScript bindings (needs: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.30)
	cd desktop && $(MACENV) wails3 generate bindings -ts -i -d frontend/bindings

dev: build helper ## run the desktop app against a running worker (hot reload needs wails3 dev)
	cd desktop/frontend && npm run build
	cd desktop && $(MACENV) CLAWDYX_WORKER=$(CURDIR)/bin/clawdyx go run .

test-desktop:
	cd desktop && $(MACENV) go vet ./... && go test ./...
	cd desktop/frontend && npm run typecheck && npm test

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

check-all: check test-desktop vet-windows ## everything, including the desktop app's tests and the Windows type check

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
	rm -rf bin coverage.out desktop/frontend/dist desktop/*.syso
