GOHOSTOS := $(shell go env GOOS)
EXE     := $(if $(filter windows,$(GOHOSTOS)),.exe,)
BIN     := bin/clawdyx$(EXE)
GOBIN   := $(shell go env GOPATH)/bin
PKG     := ./...
VERSION := 1.0.0
MACENV  := CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=14.0 CGO_CFLAGS=-mmacosx-version-min=14.0 CGO_LDFLAGS=-mmacosx-version-min=14.0
# WINARCH=arm64 builds the Windows files for arm64 (make build-windows WINARCH=arm64).
WINARCH ?= amd64
WINENV  := GOOS=windows GOARCH=$(WINARCH) CGO_ENABLED=0
LINENV  := GOOS=linux CGO_ENABLED=0
DARENV  := GOOS=darwin CGO_ENABLED=0
LINDIR  := bin/linux
# The desktop module needs cgo on macOS and Linux, not on Windows.
DESKENV := $(if $(filter darwin,$(GOHOSTOS)),$(MACENV),$(if $(filter windows,$(GOHOSTOS)),CGO_ENABLED=0,CGO_ENABLED=1))
# -race needs cgo: a Windows PC without gcc runs the tests without it.
RACE    := $(if $(filter windows,$(GOHOSTOS)),$(if $(shell command -v gcc 2>/dev/null),-race,),-race)
WINDIR  := bin/windows/clawdyX
REL     := bin/release-$(VERSION)

.PHONY: check-all release-workers release-darwin release-windows release-linux release-sums check-wording check-docs check-version check-notices check-attribution app dmg build-windows installer-windows package-linux appimage-linux helper-windows vet-windows build-linux vet-linux vet-darwin app-linux release helper frontend bindings dev test-desktop identities notices help setup run start stop restart status logs doctor build test test-integration lint fmt check-secrets check install clean

help: ## list the everyday commands
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

## Everyday commands
setup: build ## first-time setup: bot token, your Telegram ID, project folder (writes private config)
	$(BIN) setup

run: build ## run the bot in the foreground; Ctrl-C stops it
	$(BIN) run

start: install ## run the bot in the background now and at every login
	$(GOBIN)/clawdyx service start

stop: ## stop the background bot (it stays off until make start)
	@test -x $(GOBIN)/clawdyx || (echo "not installed yet; run make start"; exit 1)
	$(GOBIN)/clawdyx service stop

restart: install ## rebuild, reinstall and restart the background bot
	$(GOBIN)/clawdyx service restart

status: ## is the bot running?
	@test -x $(GOBIN)/clawdyx || (echo "not installed yet; run make start"; exit 1)
	$(GOBIN)/clawdyx service status

logs: build ## follow the bot log
	tail -n 50 -f "$$($(BIN) paths logs)/bot.log"

doctor: build ## read-only checks: config, token, Claude CLI, auth, Telegram
	$(BIN) check

## Development
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(BIN) ./cmd/clawdyx

## Desktop app
app: build helper frontend ## build bin/clawdyX.app (desktop app + worker + screen and camera helpers)
	cd desktop && $(MACENV) go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -X main.version=$(VERSION)" -o ../bin/clawdyX-gui .
	VERSION=$(VERSION) scripts/bundle.sh

DMG := bin/clawdyX-$(VERSION)-darwin-arm64.dmg

dmg: app ## build bin/clawdyX-$(VERSION)-darwin-arm64.dmg for installing by drag and drop
	rm -rf bin/dmg && mkdir -p bin/dmg && cp -R bin/clawdyX.app bin/dmg/ && ln -s /Applications bin/dmg/Applications
	hdiutil create -volname clawdyX -srcfolder bin/dmg -ov -format UDZO $(DMG) >/dev/null
	rm -rf bin/dmg && ls -lh $(DMG)

## Windows (cross-compiled from this Mac; no CGO, no extra toolchain)
build-windows: frontend ## build bin/windows/clawdyX/ for Windows $(WINARCH): clawdyX.exe (app), helpers/clawdyx.exe (worker), helpers/clawdyxw.exe (login launcher)
	rm -rf bin/windows && mkdir -p $(WINDIR)/helpers
	$(WINENV) go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(WINDIR)/helpers/clawdyx.exe ./cmd/clawdyx
	$(WINENV) go build -trimpath -ldflags="-H windowsgui -X main.version=$(VERSION)" -o $(WINDIR)/helpers/clawdyxw.exe ./cmd/clawdyxw
	@if command -v wails3 >/dev/null; then \
		sed 's/__VERSION__/$(VERSION)/g' desktop/build/windows/info.json > bin/windows/info.json && \
		sed 's/__VERSION__/$(VERSION)/g' desktop/build/windows/clawdyX.exe.manifest > bin/windows/clawdyX.exe.manifest && \
		wails3 generate syso -arch $(WINARCH) -icon desktop/build/windows/icon.ico -manifest bin/windows/clawdyX.exe.manifest \
			-info bin/windows/info.json -out desktop/rsrc_windows_$(WINARCH).syso >/dev/null; \
	else echo "note: wails3 not installed; clawdyX.exe gets no icon or manifest (see make bindings)"; fi
	cd desktop && $(WINENV) go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui -X main.version=$(VERSION)" -o ../$(WINDIR)/clawdyX.exe .
	rm -f desktop/rsrc_windows_$(WINARCH).syso bin/windows/info.json bin/windows/clawdyX.exe.manifest
	@if [ "$(GOHOSTOS)" = windows ]; then $(MAKE) --no-print-directory helper-windows || \
		{ [ "$(WINARCH)" = arm64 ] && echo "note: no arm64 MSVC tools here; this arm64 build has no camera"; }; \
	elif [ "$(WINARCH)" = amd64 ] && [ -f bin/clawdyx-camera.exe ]; then cp bin/clawdyx-camera.exe $(WINDIR)/helpers/; \
	else echo "note: the Windows camera helper is built on Windows (make helper-windows) or in CI; this build has no camera"; fi
	cp LICENSE THIRD_PARTY_NOTICES.md README.md $(WINDIR)/
	@ls -lh $(WINDIR)/clawdyX.exe $(WINDIR)/helpers/clawdyx.exe $(WINDIR)/helpers/clawdyxw.exe

installer-windows: build-windows ## build the per-user Windows installer (NSIS: makensis) into bin/release-$(VERSION)/
	mkdir -p $(REL)
	makensis -V2 -DVERSION=$(VERSION) -DSRC="$(CURDIR)/$(WINDIR)" -DOUT="$(CURDIR)/$(REL)/clawdyX-$(VERSION)-windows-$(WINARCH)-setup.exe" desktop/build/windows/installer.nsi
	@ls -lh $(REL)/clawdyX-$(VERSION)-windows-$(WINARCH)-setup.exe

# The Windows camera helper (C++, Media Foundation) needs MSVC: it builds on a
# Windows host with the Visual Studio Build Tools, and in CI.
helper-windows: ## build bin/windows/clawdyX/helpers/clawdyx-camera.exe with MSVC (Windows only)
	powershell -NoProfile -ExecutionPolicy Bypass -File desktop/helper/windows/build.ps1 -Arch $(if $(filter arm64,$(WINARCH)),arm64,x64) -Out $(WINDIR)/helpers/clawdyx-camera.exe

vet-windows: ## type-check everything (including tests) for Windows
	$(WINENV) go vet $(PKG)
	cd desktop && $(WINENV) go vet ./...

## Linux (the worker cross-compiles; the app needs cgo and GTK, so it builds on Linux only)
build-linux: ## build the Linux worker for amd64 and arm64 into bin/linux/<arch>/clawdyx
	for a in amd64 arm64; do $(LINENV) GOARCH=$$a go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(LINDIR)/$$a/clawdyx ./cmd/clawdyx || exit 1; done
	@ls -lh $(LINDIR)/*/clawdyx

NFPM := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0

package-linux: app-linux ## build the .deb, .rpm and .tar.gz for this machine's architecture into bin/release-$(VERSION)/ (Linux only)
	mkdir -p $(REL)
	for p in deb rpm; do VERSION=$(VERSION) ARCH=$$(go env GOARCH) $(NFPM) package -f desktop/build/linux/nfpm.yaml -p $$p -t $(REL)/ || exit 1; done
	n=clawdyX-$(VERSION)-linux-$$(go env GOARCH); rm -rf bin/tar && mkdir -p bin/tar/$$n && cp -R $(LINDIR)/clawdyX/. bin/tar/$$n/ && \
		tar -C bin/tar -czf $(REL)/$$n.tar.gz $$n && rm -rf bin/tar
	@ls -lh $(REL)/*.deb $(REL)/*.rpm $(REL)/*linux*.tar.gz

appimage-linux: app-linux ## build the AppImage (Linux only; downloads linuxdeploy; needs the host's WebKitGTK at run time)
	desktop/build/linux/appimage/build.sh $(VERSION) $$(go env GOARCH)

vet-linux: ## type-check everything (including tests) for Linux; the app's cgo part only on Linux
	$(LINENV) go vet $(PKG)
	cd desktop && $(LINENV) go vet ./host

app-linux: frontend ## build bin/linux/clawdyX/ (app, worker, .desktop file, icons); runs on Linux only
	@test "$$(go env GOOS)" = linux || { echo "app-linux builds on Linux only (the app needs cgo and GTK); CI builds it"; exit 1; }
	rm -rf $(LINDIR)/clawdyX && mkdir -p $(LINDIR)/clawdyX
	CGO_ENABLED=0 go build -trimpath -ldflags="-X main.version=$(VERSION)" -o $(LINDIR)/clawdyX/clawdyx ./cmd/clawdyx
	cd desktop && CGO_ENABLED=1 go build -tags production,gtk3 -trimpath -buildvcs=false -ldflags="-w -s -X main.version=$(VERSION)" -o ../$(LINDIR)/clawdyX/clawdyX .
	cp desktop/build/linux/clawdyX.desktop LICENSE THIRD_PARTY_NOTICES.md README.md $(LINDIR)/clawdyX/
	cp -R desktop/build/linux/icons $(LINDIR)/clawdyX/
	@ls -lh $(LINDIR)/clawdyX

## macOS type check from another system (the app's cgo part only on a Mac)
vet-darwin:
	$(DARENV) go vet $(PKG)
	cd desktop && $(DARENV) go vet ./host

## Release
# Release files. Each OS builds its own app files (the macOS app and the
# Linux app need their OS; the Windows camera helper needs MSVC); the
# worker-only Linux tarballs cross-compile anywhere. The release workflow
# (.github/workflows/release.yml) runs these per OS and adds SHA256SUMS.
release: check-all ## build this computer's release files into bin/release-$(VERSION)/ with SHA256SUMS (CI builds the rest)
	rm -rf $(REL) && mkdir -p $(REL)
	$(MAKE) --no-print-directory release-workers
	@case "$(GOHOSTOS)" in \
	darwin) $(MAKE) --no-print-directory release-darwin ;; \
	windows) $(MAKE) --no-print-directory release-windows ;; \
	linux) $(MAKE) --no-print-directory release-linux ;; esac
	$(MAKE) --no-print-directory release-sums
	@echo "built here: $$(cd $(REL) && ls | grep -v SHA256SUMS | tr '\n' ' ')"
	@echo "from CI only: the files of the other systems (see docs/RELEASE.md)"

# worker-tar OS ARCH SRC: clawdyx-<version>-<os>-<arch>.tar.gz with the worker and the license files.
define worker-tar
	n=clawdyx-$(VERSION)-$(1)-$(2); rm -rf bin/tar && mkdir -p bin/tar/$$n && cp $(3) bin/tar/$$n/clawdyx && \
	cp LICENSE THIRD_PARTY_NOTICES.md README.md bin/tar/$$n/ && tar -C bin/tar -czf $(REL)/$$n.tar.gz $$n && rm -rf bin/tar
endef

release-workers: build-linux ## the worker-only Linux tarballs (amd64, arm64)
	mkdir -p $(REL)
	$(call worker-tar,linux,amd64,$(LINDIR)/amd64/clawdyx)
	$(call worker-tar,linux,arm64,$(LINDIR)/arm64/clawdyx)

release-darwin: dmg ## the macOS release files (on a Mac): the .dmg and the worker tarball
	mkdir -p $(REL)
	cp $(DMG) $(REL)/
	$(call worker-tar,darwin,arm64,bin/clawdyx)

release-windows: installer-windows ## the Windows release files (on Windows): the installer and the .zip
	mkdir -p $(REL)
	z=clawdyX-$(VERSION)-windows-$(WINARCH).zip; rm -f $(REL)/$$z; \
	bsdtar="$$(cygpath -u "$${SYSTEMROOT:-C:/Windows}" 2>/dev/null)/System32/tar.exe"; \
	if command -v zip >/dev/null; then (cd bin/windows && zip -qr -X ../../$(REL)/$$z clawdyX); \
	elif [ -x "$$bsdtar" ]; then (cd bin/windows && "$$bsdtar" -a -cf ../../$(REL)/$$z clawdyX); \
	else echo "no zip tool (zip, or Windows' tar.exe)"; exit 1; fi

release-linux: package-linux ## the Linux release files (on Linux): .deb, .rpm, the app tarball; the AppImage with appimage-linux

release-sums: ## SHA256SUMS of everything in bin/release-$(VERSION)/
	cd $(REL) && rm -f SHA256SUMS && files=$$(ls | sort) && \
	if command -v sha256sum >/dev/null; then sha256sum $$files; else shasum -a 256 $$files; fi | sed 's/ [*]/  /' > SHA256SUMS && cat SHA256SUMS

helper:
	swiftc -O -parse-as-library -target $$(uname -m)-apple-macos14.0 desktop/helper/main.swift -o bin/clawdyx-screenshot
	swiftc -O -parse-as-library -target $$(uname -m)-apple-macos14.0 desktop/helper/camera.swift -o bin/clawdyx-camera \
		-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker desktop/helper/camera-Info.plist

frontend:
	cd desktop/frontend && (test -d node_modules || npm ci) && npm run build

bindings: ## regenerate the TypeScript bindings (needs: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.30)
	cd desktop && $(DESKENV) wails3 generate bindings -ts -i -d frontend/bindings

dev: build helper ## run the desktop app against a running worker (hot reload needs wails3 dev)
	cd desktop/frontend && npm run build
	cd desktop && $(MACENV) CLAWDYX_WORKER=$(CURDIR)/bin/clawdyx go run .

test-desktop:
	cd desktop && $(DESKENV) go vet ./... && $(DESKENV) go test ./...
	cd desktop/frontend && npm run typecheck && npm test

test:
	go test $(RACE) $(PKG)

test-integration:
	go test $(RACE) -tags integration $(PKG)

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

check-docs: ## per-OS sections of the docs name only their OS
	sh scripts/check-docs.sh

check-wording: ## fail when user-facing text tells people to edit config.json
	@scripts/check-wording.sh

check-version: ## VERSION is CHANGELOG.md's first entry and has release notes (TAG=vX.Y.Z also checks a tag)
	@v=$$(sed -n 's/^## \([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\) .*/\1/p' CHANGELOG.md | head -1); \
	[ "$$v" = "$(VERSION)" ] || { echo "CHANGELOG.md starts with $$v, the Makefile says $(VERSION)"; exit 1; }; \
	[ -f desktop/build/release-notes/$(VERSION).md ] || { echo "missing desktop/build/release-notes/$(VERSION).md"; exit 1; }; \
	[ -z "$(TAG)" ] || [ "$(TAG)" = "v$(VERSION)" ] || { echo "the tag $(TAG) is not v$(VERSION)"; exit 1; }; \
	echo "version $(VERSION) ok"

check-notices: ## THIRD_PARTY_NOTICES.md is up to date (needs desktop/frontend/node_modules)
	@mkdir -p bin && sh scripts/gen-notices.sh bin/notices.md >/dev/null && \
	if diff -u THIRD_PARTY_NOTICES.md bin/notices.md >/dev/null; then echo "notices up to date"; rm -f bin/notices.md; \
	else diff -u THIRD_PARTY_NOTICES.md bin/notices.md | head -40; echo "run scripts/gen-notices.sh and commit THIRD_PARTY_NOTICES.md"; exit 1; fi

check-attribution: ## no commit names Claude as an author or co-author
	@sh scripts/check-attribution.sh

check: lint test check-secrets

check-all: check check-wording check-docs check-version check-attribution test-desktop vet-windows vet-linux vet-darwin ## everything: tests, the desktop app's tests and the type checks for all three OSes

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
