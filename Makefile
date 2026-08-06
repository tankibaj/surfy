VERSION ?= 0.1.0

.PHONY: build build-apple-silicon install-user dist test check

build:
	mkdir -p bin
	GOCACHE=$${GOCACHE:-/tmp/serve-them-go-cache} go build -o bin/serve-them ./cmd/serve-them

build-apple-silicon:
	mkdir -p bin
	GOCACHE=$${GOCACHE:-/tmp/serve-them-go-cache} CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/serve-them-darwin-arm64 ./cmd/serve-them

install-user: build-apple-silicon
	mkdir -p $${HOME}/.local/bin
	install -m 0755 bin/serve-them-darwin-arm64 $${HOME}/.local/bin/serve-them

dist: build-apple-silicon
	mkdir -p dist/stage
	cp bin/serve-them-darwin-arm64 dist/stage/serve-them
	tar -C dist/stage -czf dist/serve-them_$${VERSION:-$(VERSION)}_darwin_arm64.tar.gz serve-them
	shasum -a 256 dist/serve-them_$${VERSION:-$(VERSION)}_darwin_arm64.tar.gz

test:
	GOCACHE=$${GOCACHE:-/tmp/serve-them-go-cache} go test ./...

check:
	gofmt -w cmd internal
	GOCACHE=$${GOCACHE:-/tmp/serve-them-go-cache} go vet ./...
	GOCACHE=$${GOCACHE:-/tmp/serve-them-go-cache} go test ./...
