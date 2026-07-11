# Build and install orma.
# Source version stays "dev"; release builds stamp VERSION via ldflags.
#
# ONNX MiniLM needs cgo (default on). Cross pure-Go builds use CGO_ENABLED=0
# and fall back to the hash embedder until users download ORT + rebuild with cgo.

PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
VERSION ?= dev
LDFLAGS := -s -w -X github.com/anandh8x/orma/internal/cli.version=$(VERSION)
DIST    ?= dist

.PHONY: build install test clean run-init release-binaries

build:
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/orma ./cmd/orma

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 bin/orma "$(BINDIR)/orma"
	@echo "installed $(BINDIR)/orma"
	@echo "ensure $(BINDIR) is on PATH, then: orma init"

test:
	go test ./...

clean:
	rm -rf bin dist

run-init: install
	"$(BINDIR)/orma" init --skip-history
	"$(BINDIR)/orma" doctor

# Build release tarballs into dist/
# - linux/amd64: cgo enabled (full ONNX MiniLM support on typical Linux hosts)
# - others: CGO_ENABLED=0 (hash embedder; go install with cgo for ONNX)
# Usage: make release-binaries VERSION=v0.1.0
release-binaries:
	@test -n "$(VERSION)" || (echo "VERSION required, e.g. VERSION=v0.1.0" >&2; exit 1)
	@mkdir -p "$(DIST)"
	@rm -f "$(DIST)"/*.tar.gz
	@echo "building orma_linux_amd64 (cgo)"
	@go build -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST)/orma" ./cmd/orma
	@tar -C "$(DIST)" -czf "$(DIST)/orma_linux_amd64.tar.gz" orma
	@rm -f "$(DIST)/orma"
	@for pair in linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$${pair%/*}; arch=$${pair#*/}; \
		out="orma_$${os}_$${arch}"; \
		echo "building $$out (CGO_ENABLED=0)"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST)/orma" ./cmd/orma; \
		tar -C "$(DIST)" -czf "$(DIST)/$${out}.tar.gz" orma; \
		rm -f "$(DIST)/orma"; \
	done
	@ls -lh "$(DIST)"
