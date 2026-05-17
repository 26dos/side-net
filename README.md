# side-net

Verified retrieval cache pipeline for building and serving Merkle side-node
proofs.

This repo builds a cache layer for large content-addressed objects. Once a full
object has been retrieved and its Merkle tree has been computed, later clients
can verify partial retrievals by combining the requested byte range with cached
side nodes instead of downloading the full object again.

The current implementation is Filecoin-compatible, using Fr32 and Poseidon
parameters, but the engineering pattern is a general verified-cache pipeline:
consume successful retrieval records, build proof artifacts, store them, and let
future reads validate against a known root.

## Pipeline

```
retrieval result queue
      |
      v
full-tree worker
      |
      +--> parse provider addresses
      +--> download source object
      +--> run Merkle tree builder
      +--> validate computed root
      |
      v
side-node / window-path store
      |
      v
partial retrieval verification
```

## Components

- **Go worker** in `cmd/full-tree-worker`
  - polls successful retrieval records
  - downloads the source object
  - invokes the tree-building CLI
  - writes proof artifacts to MongoDB

- **Rust CLI** in `cmd/piece-tree-cli`
  - reads raw bytes from stdin
  - maps chunks into field elements
  - builds a Poseidon Merkle tree
  - emits window paths as JSON

- **Mongo collections**
  - `claims_task_result`: input records from retrieval jobs
  - `side_window_paths`: cached proof artifacts for later verification

## Output Shape

The tree builder emits JSON like:

```json
{
  "hash_algo": "poseidon-filecoin",
  "arity": 8,
  "leaf_size": 32,
  "total_leaves": "...",
  "root": "hex32",
  "window_size_bytes": 1048576,
  "window_paths": [
    {
      "window_id": 0,
      "start_leaf": 0,
      "leaf_count": 32768,
      "siblings": [["hex32", "..."]]
    }
  ]
}
```

## Build

```bash
make build-rust
make build-go
```

Manual build:

```bash
cd cmd/piece-tree-cli
cargo build --release

cd ../..
go mod tidy
go build -o bin/full-tree-worker ./cmd/full-tree-worker
```

## Run

```bash
export MONGO_URI="mongodb://127.0.0.1:27017"
export MONGO_DB="fil"

./bin/full-tree-worker
```

## Operational Notes

- Workers should shard by object id or use an explicit work queue at scale.
- Failed downloads should be marked with structured error codes.
- Invalid tree-builder output should be treated as a build error.
- Window size is 1 MiB by default.
- Hot proof artifacts can be replicated to Redis, object storage, or edge POPs.
- Cache data is untrusted; incorrect siblings fail root validation.

## Why This Repo Belongs In The Portfolio

`side-net` shows low-level data infrastructure work:

- streaming downloads into compute jobs
- cross-language worker orchestration
- reproducible proof artifact generation
- cache design for partial reads
- validation-first storage of derived data
- operational failure handling around expensive jobs

## License

Apache-2.0.
