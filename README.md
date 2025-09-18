# Side-Net: Filecoin-Compatible Side-Node Cache Network

This project builds a **Side-Node caching network** for Filecoin pieces so that once a full Merkle tree
(Fr32 + Poseidon, arity=8) has been computed by anyone, later verifiers/retrievers can avoid re-reading the
entire piece: they download only the **target window data** plus **O(log N) × 32B** siblings from the cache.

## Architecture

```
claims_task_result (Mongo, producer)
      │ (success=true, http module)
      ▼
full-tree-worker (Go)
  ├─ parse provider multiaddrs → ip:port
  ├─ download piece: http://{ip}:{port}/piece/{cid}
  ├─ invoke piece-tree-cli (Rust)
  │     • Fr32 chunking (32B) → Poseidon (arity 8) Merkle
  │     • export window paths (siblings from window-root to piece-root)
  └─ store JSON into Mongo collection: side_window_paths
            (piece, built_at, root, arity, leaf_size, window_size_bytes, paths)
```

## Full-Tree Construction (Overview)

- **Leaves**: map 32-byte chunks of the piece into field elements (**Fr32**) and use them as tree leaves.
- **Hasher**: Filecoin uses **Poseidon** over the BLS12-381 scalar field with arity **8**.
- **Padding / Power-of-arity**: the number of leaves is padded up to a multiple of 8^k so the final root is well-defined.
- **Window Path**: for a window (e.g., 1 MiB = 1<<20 bytes), we compute the siblings on each level from the **window root** up to the **piece root**.  
  Those siblings are cached in Mongo so that provers can combine their locally-calculated lower part with the cached siblings to get the root.

> **Note**: For *bit-accurate* production parity with Lotus miners, integrate the exact Fr32 packing pipeline used by
> `rust-fil-proofs`. This CLI maps 32B chunks to `Fr` using `bytes_into_fr` and is suitable for building a compatible tree
> and window proofs in most practical scenarios. Replace with end-to-end sector pipelines if your environment requires strict bit-level compatibility.

## Repositories / Components

- **Rust CLI** `cmd/piece-tree-cli`: reads raw piece bytes from **stdin**, outputs JSON with:
  ```json
  {
    "hash_algo":"poseidon-filecoin",
    "arity":8,
    "leaf_size":32,
    "total_leaves": "...",
    "root": "hex32",
    "window_size_bytes": 1048576,
    "window_paths":[
      {"window_id":0,"start_leaf":0,"leaf_count":..., "siblings":[["hex32","..."], ["..."]]} , ...
    ]
  }
  ```
- **Go Worker** `cmd/full-tree-worker`: polls `claims_task_result` (success=true, module=http), downloads the piece from the provider’s HTTP endpoint derived from multiaddrs, pipes it to the Rust CLI, then upserts to `side_window_paths`. If download/build fails, it marks the record as `result.success=false` with an error code/message.

## Dependencies

- **Rust** 1.68+ (Cargo)  
- **Go** 1.22+  
- **MongoDB** 5.x+  
- Linux/macOS (recommended; Windows WSL works as well)

### Rust crates
- `storage-proofs-core` (from `rust-fil-proofs`)
- `neptune`, `blstrs`, `serde`, `serde_json`, `anyhow`, `hex`

### Go modules
- `go.mongodb.org/mongo-driver`
- `github.com/multiformats/go-multiaddr`

## Build

### Makefile
```bash
make build-rust
make install-rust     # installs /usr/local/bin/piece-tree-cli
make build-go         # builds ./bin/full-tree-worker
```

### Manual
```bash
# Rust
cd cmd/piece-tree-cli
cargo build --release
sudo install -m 0755 target/release/piece-tree-cli /usr/local/bin/piece-tree-cli

# Go
cd ../..
go mod tidy
go build -o bin/full-tree-worker ./cmd/full-tree-worker
```

## Configuration & Deployment

### Environment Variables (Go Worker)
```bash
export MONGO_URI="mongodb://127.0.0.1:27017"
export MONGO_DB="fil"
# Optional: adjust ticker interval in code if needed
```

### Mongo Collections
- **claims_task_result**: input queue (produced by your retrieval program).
- **side_window_paths**: output cache for window paths:
  ```json
  {
    "piece": "baga...",
    "built_at": "...",
    "hash_algo": "poseidon-filecoin",
    "arity": 8,
    "leaf_size": 32,
    "root": "hex32",
    "window_size_bytes": 1048576,
    "paths": [ ... ],
    "meta": { "impl": "poseidon-filecoin" }
  }
  ```

### Run
```bash
# Ensure piece-tree-cli is available in PATH
which piece-tree-cli

# Start worker
./bin/full-tree-worker
# or
go run ./cmd/full-tree-worker
```

## Operational Notes

- The worker picks the **most recent** `success=true` entry. For scale-out, shard by piece CID or add a small work-queue to avoid duplicate processing.
- If the HTTP download returns non-200 or stream fails, the record is marked failed with `download_error`.
- If the CLI fails or outputs invalid JSON, the record is marked failed with `build_error`.
- Window size is set to **1 MiB** by default; tune in the Rust CLI as needed.
- For production, consider:
  - Storing per-node `(piece, level, index) -> node_hash` to enable `GET /sidenodes/{piece}/{level}/{index}` style serving.
  - Adding gRPC/HTTP read-only APIs to expose the cache.
  - Replicating hot pieces to Redis/edge POPs/CDN for low latency.

## Security

No trust required in the side-node: wrong siblings yield a root mismatch against the on-chain CommP/PieceCID and verification fails deterministically.

## License

Apache-2.0 (adjust to your org’s policy).
