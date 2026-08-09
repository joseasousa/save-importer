.PHONY: test build-arm64 package

test:
	go test ./internal/...

# Requires an aarch64 Linux cross-compiler with SDL2 development files.
build-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc go build -trimpath -ldflags="-s -w" -o build/muos-save-importer ./cmd/importer

package: build-arm64
	pwsh -File scripts/package.ps1 -Binary build/muos-save-importer
