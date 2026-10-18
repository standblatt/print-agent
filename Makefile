VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
# The server image of the contract test; the CI workflow is the only place it is set.
SERVER_VERSION := $(shell sed -n 's/^ *SERVER_VERSION: //p' .github/workflows/ci.yml)
CONTRACT := standblatt-contract

.PHONY: build prepare test clean

# The headless agent for this computer and the Linux release binaries.
build:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/standblatt-print-agent ./cmd/standblatt-print-agent
	for arch in amd64 arm64; do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
			-o bin/standblatt-print-agent-linux-$$arch ./cmd/standblatt-print-agent || exit 1; \
	done

# Everything to check before a commit: gofmt, go vet and staticcheck (also for Windows), doc comments.
prepare:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }
	go vet ./...
	GOOS=windows go vet ./...
	go tool staticcheck ./...
	go build -o bin/staticcheck honnef.co/go/tools/cmd/staticcheck
	GOOS=windows bin/staticcheck ./...
	python3 scripts/funcdoc.py
	@git status --short

# Go tests, then the contract test against the server image of the CI with PostgreSQL in Docker (server on :18080).
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
		BASE_URL=http://localhost:18080 AGENT=bin/standblatt-print-agent scripts/contract.sh \
			|| { docker logs $(CONTRACT)-server 2>&1 | tail -20; exit 1; }

# Removes build output and leftover contract test containers.
clean:
	@docker rm -f $(CONTRACT)-db $(CONTRACT)-server >/dev/null 2>&1 || true
	@docker network rm $(CONTRACT) >/dev/null 2>&1 || true
	rm -rf bin
