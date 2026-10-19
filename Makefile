VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
# The server image of the contract test, from its tag in testdata/contract/Dockerfile (raised by Dependabot).
SERVER_VERSION := $(shell sed -n 's/^FROM .*://p' testdata/contract/Dockerfile)
CONTRACT := standblatt-contract

GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
APP_LDFLAGS := -s -w -X main.version=$(VERSION)
MAC_APP := Standblatt Print Agent.app
# Package version: a release tag without the v, else 0.0.0.
PKG_VERSION := $(or $(shell echo $(VERSION) | sed -n 's/^v\([0-9][0-9.]*\)$$/\1/p'),0.0.0)

# All builds go to bin/.
.PHONY: app app-mac app-dmg app-linux app-linux-packages app-windows app-installer app-headless app-deps app-frontend dev prepare test clean

# The desktop app for the operating system of this computer.
app: app-$(subst darwin,mac,$(GOOS))

# The desktop app for this Mac (CUPS backend) as a bundle, signed ad hoc so it runs here: bin/$(MAC_APP).
# GOARCH=amd64 builds it for Intel Macs.
app-mac: app-frontend
	cd app && CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=12.0 CGO_CFLAGS=-mmacosx-version-min=12.0 CGO_LDFLAGS=-mmacosx-version-min=12.0 \
		go build -tags production -trimpath -ldflags="$(APP_LDFLAGS)" -o ../bin/standblatt-print-agent-app-darwin-$(GOARCH) .
	cd bin && rm -rf "$(MAC_APP)" && mkdir -p "$(MAC_APP)/Contents/MacOS" "$(MAC_APP)/Contents/Resources" && \
		cp standblatt-print-agent-app-darwin-$(GOARCH) "$(MAC_APP)/Contents/MacOS/standblatt-print-agent" && \
		cp ../app/build/darwin/icons.icns "$(MAC_APP)/Contents/Resources/" && \
		sed 's/@VERSION@/$(VERSION)/' ../app/build/darwin/Info.plist > "$(MAC_APP)/Contents/Info.plist" && \
		codesign --force --deep --sign - "$(MAC_APP)"

# The Mac app as a disk image with a link to Applications to drag it into, and zipped for the in-app updater.
app-dmg: app-mac
	rm -rf bin/dmg && mkdir bin/dmg && cp -R "bin/$(MAC_APP)" bin/dmg/ && ln -s /Applications bin/dmg/Applications
	hdiutil create -volname "Standblatt Print Agent" -srcfolder bin/dmg -ov -format UDZO \
		bin/standblatt-print-agent-app-darwin-$(GOARCH).dmg
	rm -rf bin/dmg
	ditto -c -k --keepParent "bin/$(MAC_APP)" bin/standblatt-print-agent-app-darwin-$(GOARCH).app.zip

# The desktop app for Linux (CUPS backend) on this architecture. It needs CGO with the GTK 4 and WebKitGTK 6.0
# development packages, so it builds on Linux only.
app-linux: app-frontend
	cd app && CGO_ENABLED=1 go build -tags production -trimpath -ldflags="$(APP_LDFLAGS)" -o ../bin/standblatt-print-agent-app-linux-$(GOARCH) .

# The Linux app as .deb and .rpm (with fpm): binary, menu entry and icon.
app-linux-packages: app-linux
	rm -rf bin/pkg && mkdir -p bin/pkg/usr/bin bin/pkg/usr/share/applications bin/pkg/usr/share/icons/hicolor/512x512/apps
	cp bin/standblatt-print-agent-app-linux-$(GOARCH) bin/pkg/usr/bin/standblatt-print-agent-app
	cp app/build/linux/standblatt-print-agent.desktop bin/pkg/usr/share/applications/
	cp app/build/appicon.png bin/pkg/usr/share/icons/hicolor/512x512/apps/standblatt-print-agent.png
	for t in deb rpm; do \
		if [ $$t = deb ]; then deps="-d libgtk-4-1 -d libwebkitgtk-6.0-4 -d cups-client"; \
		else deps="-d gtk4 -d webkitgtk6.0 -d cups-client"; fi; \
		fpm -s dir -t $$t -f -C bin/pkg -n standblatt-print-agent-app -v $(PKG_VERSION) -a $(GOARCH) $$deps \
			--license AGPL-3.0 --url https://github.com/standblatt/print-agent --maintainer "Standblatt" \
			--description "Prints the documents of a Standblatt range on the printers of this computer." \
			-p bin/ usr || exit 1; \
	done
	rm -rf bin/pkg

# The desktop app for Windows (amd64), cross-compiled without CGO, with icon and manifest from a syso file.
app-windows: app-frontend
	cd app && go tool wails3 generate syso -arch amd64 -icon build/windows/icon.ico \
		-manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_amd64.syso
	cd app && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags production -trimpath -ldflags="$(APP_LDFLAGS) -H windowsgui" \
		-o ../bin/standblatt-print-agent-app-windows-amd64.exe .; s=$$?; rm -f wails_windows_amd64.syso; exit $$s

# The Windows installer: per user without admin rights, start menu and desktop shortcut, autostart (needs makensis).
app-installer: app-windows
	cd app && GOOS= GOARCH= ../bin/wails3 generate webview2bootstrapper -dir ../bin/webview2
	makensis -WX -DVERSION=$(PKG_VERSION) -DEXE=$(CURDIR)/bin/standblatt-print-agent-app-windows-amd64.exe \
		-DBOOTSTRAPPER=$(CURDIR)/bin/webview2/MicrosoftEdgeWebview2Setup.exe \
		-DOUT=$(CURDIR)/bin/Standblatt-Print-Agent-Setup.exe app/build/windows/installer.nsi

# The headless agent (CUPS) for this computer and the Linux release binaries.
app-headless:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/standblatt-print-agent ./cmd/standblatt-print-agent
	for arch in amd64 arm64; do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
			-o bin/standblatt-print-agent-linux-$$arch ./cmd/standblatt-print-agent || exit 1; \
	done

# Frontend packages and the TypeScript bindings of the app's Go services. The wails3 tool is built for this
# computer, whatever GOOS and GOARCH a build is for, and reads the app as a Windows build: the bindings are the same
# on every system, and Wails needs no CGO there (on Linux it would need GTK).
app-deps:
	cd app/frontend && npm install --no-audit --no-fund
	cd app && GOOS= GOARCH= go build -o ../bin/wails3 github.com/wailsapp/wails/v3/cmd/wails3
	cd app && GOOS=windows GOARCH=amd64 ../bin/wails3 generate bindings -clean=true -ts -i

# The frontend the app embeds.
app-frontend: app-deps
	cd app/frontend && npm run build

# The app in a window on this computer (CUPS backend, real config and printers) through wails3 dev: Vite serves the
# frontend, Go changes in app/ rebuild and restart the app. A dev build never turns on autostart and logs to the
# terminal too.
dev: app-deps
	cd app && go tool wails3 dev -config build/config.yml

# Everything to check before a commit: gofmt, go vet and staticcheck (also for Windows) of both modules,
# svelte-check, doc comments.
prepare: app-deps
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }
	go vet ./...
	GOOS=windows go vet ./...
	go tool staticcheck ./...
	go build -o bin/staticcheck honnef.co/go/tools/cmd/staticcheck
	GOOS=windows bin/staticcheck ./...
	cd app && go vet ./... && GOOS=windows go vet ./... && ../bin/staticcheck ./... && GOOS=windows ../bin/staticcheck ./...
	cd app/frontend && npm run check
	python3 scripts/funcdoc.py
	@git status --short

# Go tests, then the contract tests (agent binary, pairing) against the server image of the CI with PostgreSQL in
# Docker (server on :18080).
test:
	go test ./...
	go build -o bin/standblatt-print-agent ./cmd/standblatt-print-agent
	@set -e; trap 'docker rm -f $(CONTRACT)-db $(CONTRACT)-server >/dev/null 2>&1; docker network rm $(CONTRACT) >/dev/null 2>&1' EXIT; \
		docker network create $(CONTRACT) >/dev/null; \
		docker run -d --name $(CONTRACT)-db --network $(CONTRACT) -e POSTGRES_USER=standblatt -e POSTGRES_PASSWORD=standblatt \
			-e POSTGRES_DB=standblatt postgres:18-alpine >/dev/null; \
		until docker exec $(CONTRACT)-db pg_isready -q -U standblatt; do sleep 1; done; \
		docker run -d --platform linux/amd64 --name $(CONTRACT)-server --network $(CONTRACT) -p 18080:8080 -e STANDBLATT_METRICS_LISTEN= \
			-e DATABASE_URL=postgres://standblatt:standblatt@$(CONTRACT)-db:5432/standblatt ghcr.io/standblatt/server:$(SERVER_VERSION) >/dev/null; \
		for i in $$(seq 60); do curl -sf -o /dev/null http://localhost:18080/readyz && break; sleep 1; done; \
		curl -sSf https://raw.githubusercontent.com/standblatt/server/$(SERVER_VERSION)/docker/dev-seed.sql | \
			docker exec -i $(CONTRACT)-db psql -q -U standblatt -d standblatt >/dev/null; \
		{ BASE_URL=http://localhost:18080 AGENT=bin/standblatt-print-agent scripts/contract.sh && \
			STANDBLATT_CONTRACT_URL=http://localhost:18080 go test ./internal/agent -run Contract -count=1; } \
			|| { docker logs $(CONTRACT)-server 2>&1 | tail -20; exit 1; }

# Removes build output and leftover contract test containers.
clean:
	@docker rm -f $(CONTRACT)-db $(CONTRACT)-server >/dev/null 2>&1 || true
	@docker network rm $(CONTRACT) >/dev/null 2>&1 || true
	rm -rf bin
