VERSION ?= 0.2.0

.PHONY: build build-apple-silicon install-user dist test check

build:
	mkdir -p bin
	GOCACHE=$${GOCACHE:-/tmp/surfy-go-cache} go build -o bin/surfy ./cmd/surfy

build-apple-silicon:
	mkdir -p bin
	GOCACHE=$${GOCACHE:-/tmp/surfy-go-cache} CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/surfy-darwin-arm64 ./cmd/surfy

install-user: build-apple-silicon
	mkdir -p $${HOME}/.local/bin
	install -m 0755 bin/surfy-darwin-arm64 $${HOME}/.local/bin/surfy

dist: build-apple-silicon
	mkdir -p dist/stage
	cp bin/surfy-darwin-arm64 dist/stage/surfy
	tar -C dist/stage -czf dist/surfy_$${VERSION:-$(VERSION)}_darwin_arm64.tar.gz surfy
	shasum -a 256 dist/surfy_$${VERSION:-$(VERSION)}_darwin_arm64.tar.gz

test:
	GOCACHE=$${GOCACHE:-/tmp/surfy-go-cache} go test ./...

check:
	gofmt -w cmd internal
	GOCACHE=$${GOCACHE:-/tmp/surfy-go-cache} go vet ./...
	GOCACHE=$${GOCACHE:-/tmp/surfy-go-cache} go test ./...
