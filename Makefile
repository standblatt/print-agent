VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
# The server image of the contract test; the CI workflow is the only place it is set.
SERVER_VERSION := $(shell sed -n 's/^ *SERVER_VERSION: //p' .github/workflows/ci.yml)
CONTRACT := standblatt-contract

GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
APP_LDFLAGS := -s -w -X main.version=$(VERSION)
MAC_APP := Standblatt Print Agent.app

# All builds go to bin/.
.PHONY: app app-mac app-linux app-windows app-headless app-deps app-frontend dev prepare test clean

# The desktop app for the operating system of this computer.
app: app-$(subst darwin,mac,$(GOOS))

# The desktop app for this Mac (CUPS backend) as a bundle, signed ad hoc so it runs here: bin/$(MAC_APP).
app-mac: app-frontend
	cd app && MACOSX_DEPLOYMENT_TARGET=12.0 CGO_CFLAGS=-mmacosx-version-min=12.0 CGO_LDFLAGS=-mmacosx-version-min=12.0 \
		go build -tags production -trimpath -ldflags="$(APP_LDFLAGS)" -o ../bin/standblatt-print-agent-app-darwin-$(GOARCH) .
	cd bin && rm -rf "$(MAC_APP)" && mkdir -p "$(MAC_APP)/Contents/MacOS" "$(MAC_APP)/Contents/Resources" && \
		cp standblatt-print-agent-app-darwin-$(GOARCH) "$(MAC_APP)/Contents/MacOS/standblatt-print-agent" && \
		cp ../app/build/darwin/icons.icns "$(MAC_APP)/Contents/Resources/" && \
		sed 's/@VERSION@/$(VERSION)/' ../app/build/darwin/Info.plist > "$(MAC_APP)/Contents/Info.plist" && \
		codesign --force --deep --sign - "$(MAC_APP)"

# The desktop app for Linux (CUPS backend) on this architecture. It needs CGO with the GTK 4 and WebKitGTK 6.0
# development packages, so it builds on Linux only.
app-linux: app-frontend
	cd app && CGO_ENABLED=1 go build -tags production -trimpath -ldflags="$(APP_LDFLAGS)" -o ../bin/standblatt-print-agent-app-linux-$(GOARCH) .

# The desktop app for Windows (amd64), cross-compiled without CGO, with icon and manifest from a syso file.
app-windows: app-frontend
	cd app && go tool wails3 generate syso -arch amd64 -icon build/windows/icon.ico \
		-manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_amd64.syso
	cd app && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags production -trimpath -ldflags="$(APP_LDFLAGS) -H windowsgui" \
		-o ../bin/standblatt-print-agent-app-windows-amd64.exe .; s=$$?; rm -f wails_windows_amd64.syso; exit $$s

# The headless agent (CUPS) for this computer and the Linux release binaries.
app-headless:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/standblatt-print-agent ./cmd/standblatt-print-agent
	for arch in amd64 arm64; do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
			-o bin/standblatt-print-agent-linux-$$arch ./cmd/standblatt-print-agent || exit 1; \
	done

# Frontend packages and the TypeScript bindings of the app's Go services.
app-deps:
	cd app/frontend && npm install --no-audit --no-fund
	cd app && go tool wails3 generate bindings -clean=true -ts -i

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
