# Build and install a runnable orma binary.
# Product version stays "dev" in source; only release builds stamp VERSION.

PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
VERSION ?= dev
LDFLAGS := -s -w -X github.com/anandh8x/orma/internal/cli.version=$(VERSION)

.PHONY: build install test clean run-init

build:
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/orma ./cmd/orma

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 bin/orma "$(BINDIR)/orma"
	@echo "installed $(BINDIR)/orma"
	@echo "ensure $(BINDIR) is on PATH, then:"
	@echo "  orma init"
	@echo "  eval \"\$$(orma hook zsh)\"   # or bash"

test:
	go test ./...

clean:
	rm -rf bin

run-init: install
	"$(BINDIR)/orma" init --skip-history
	"$(BINDIR)/orma" doctor
