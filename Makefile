APP_NAME := jalon
BIN := bin/$(APP_NAME)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO_LDFLAGS := -s -w -X main.version=$(VERSION)

# One line per published platform. Adding one is a one word change, and the
# release is reproducible on a laptop: CI only calls this Makefile.
RELEASE_PLATFORMS := \
	linux/amd64 linux/arm64 linux/arm linux/386 \
	darwin/amd64 darwin/arm64 \
	windows/amd64 windows/arm64 \
	freebsd/amd64

.PHONY: fmt vet staticcheck test bench check build install site dogfood release brew-formula clean

# --- Dev loop ---

fmt:
	@out=$$(gofmt -l . 2>&1); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

# Not part of `check`: it needs a tool that is not in the Go distribution, and a
# missing tool must say so rather than be skipped quietly. CI runs it as its own
# job. Rebuild it after a Go upgrade, it refuses modules newer than itself.
staticcheck:
	@command -v staticcheck >/dev/null 2>&1 || { \
		echo "staticcheck is not installed: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck ./...

test:
	go test -race -count=1 ./...

# The measurement behind "render is a full rewrite every time".
bench:
	go test -run XXX -bench BenchmarkRenderSite -benchtime 5x .

check: fmt vet test dogfood

build:
	mkdir -p bin
	go build -trimpath -ldflags "$(GO_LDFLAGS)" -o ./$(BIN) .

install:
	go install -trimpath -ldflags "$(GO_LDFLAGS)" .

# Regenerate this repository's own task view.
site: build
	./$(BIN) render

# jalon runs against its own .tasks/: the repository is the regression corpus.
# A format change that breaks real files fails here before it reaches anyone.
dogfood: build
	@set -e; \
	for f in .tasks/*.md; do \
		id=$$(basename "$$f" .md); \
		./$(BIN) digest -offline "$$id" >/dev/null; \
		./$(BIN) compact -check -max-tokens 5000 "$$id" >/dev/null; \
	done; \
	./$(BIN) render >/dev/null; \
	echo "dogfood: $$(ls .tasks/*.md | wc -l | tr -d ' ') task(s) digested, checked and rendered"

# --- Release ---

# Builds every platform into dist/, one archive plus one checksum file each.
# CGO is off, so every target cross compiles from any host.
release: clean
	@mkdir -p dist
	@set -e; \
	sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$$@"; else shasum -a 256 "$$@"; fi; }; \
	for p in $(RELEASE_PLATFORMS); do \
		goos=$${p%/*}; goarch=$${p#*/}; \
		bin=$(APP_NAME); [ "$$goos" = "windows" ] && bin=$(APP_NAME).exe || true; \
		name=$(APP_NAME)-$$goos-$$goarch-$(VERSION); \
		echo "-> $$goos/$$goarch"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch \
			go build -trimpath -ldflags "$(GO_LDFLAGS)" -o "dist/$$bin" .; \
		if [ "$$goos" = "windows" ]; then \
			( cd dist && zip -q "$$name.zip" "$$bin" && rm "$$bin" ); asset=$$name.zip; \
		else \
			( cd dist && tar -czf "$$name.tar.gz" "$$bin" && rm "$$bin" ); asset=$$name.tar.gz; \
		fi; \
		( cd dist && sha "$$asset" > "$$asset"_checksums.txt ); \
	done
	@ls -1 dist

# Writes the Homebrew formula from the checksums that release just produced.
# There is no tap yet on purpose: a tap is a second repository to operate and a
# formula to resynchronize at every release, for nobody. docs/release.md holds
# the five minute procedure for the day someone asks.
brew-formula:
	@test -d dist || { echo "run make release first"; exit 1; }
	@VERSION=$(VERSION) sh packaging/homebrew/generate.sh > packaging/homebrew/$(APP_NAME).rb
	@echo "wrote packaging/homebrew/$(APP_NAME).rb for $(VERSION)"

clean:
	rm -rf bin/* dist .tasks/site
