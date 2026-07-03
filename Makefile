# Canonical Go Makefile — fjacquet/ci standard interface (do not rename targets).
# This module is a LIBRARY: no binary, no release, no Docker targets.
.DEFAULT_GOAL := all
COVER   ?= coverage.out
DIST    ?= dist

# Pinned tool versions (installed by `make tools`).
GOLANGCI_VERSION ?= v2.12.2

.PHONY: all clean install tools lint format test build vuln security sbom \
        coverage-upload ci fmt-check fmt vet test-race test-coverage sure

all: clean lint test build

clean:
	rm -f $(COVER) coverage.html *.sarif
	rm -rf $(DIST)

install:
	go mod download

# Install pinned dev/CI tooling into $GOPATH/bin.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@latest

lint:
	golangci-lint run --timeout=5m

format:
	golangci-lint fmt

test:
	go test -race -coverprofile=$(COVER) -covermode=atomic ./...

build:
	go build ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

security:  # advisory: reports findings but never blocks the build (CodeQL/osv are the blocking gates)
	uvx semgrep scan --config auto --skip-unknown-extensions || true

# Software Bill of Materials (CycloneDX) — required by the shared go-ci workflow.
sbom:
	mkdir -p $(DIST)
	go run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest mod -json -output $(DIST)/sbom.cdx.json

# Upload coverage to Codecov — required by the shared go-ci workflow (never blocks).
coverage-upload:
	uvx --from codecov-cli codecov upload-process --file $(COVER) || true

# Aggregate gate run by CI.
ci: lint test build vuln

# --- repo-specific convenience targets ---

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed in:"; gofmt -l .; exit 1)

fmt:
	go fmt ./...

vet:
	go vet ./...

test-race: test

test-coverage: test
	go tool cover -html=$(COVER) -o coverage.html

# Local convenience: format, vet, test, build, lint.
sure: fmt vet test
	go build ./...
	golangci-lint run
