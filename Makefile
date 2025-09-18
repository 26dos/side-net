RUST_BIN=piece-tree-cli
GO_WORKER=full-tree-worker

.PHONY: build-rust install-rust build-go clean

build-rust:
	cd cmd/piece-tree-cli && cargo build --release

install-rust: build-rust
	install -m 0755 cmd/piece-tree-cli/target/release/$(RUST_BIN) /usr/local/bin/$(RUST_BIN)

build-go:
	go mod tidy
	mkdir -p bin
	go build -o bin/$(GO_WORKER) ./cmd/$(GO_WORKER)

clean:
	rm -rf bin
	cd cmd/piece-tree-cli && cargo clean || true
