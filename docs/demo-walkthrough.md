# Verified Retrieval Cache Demo

This walkthrough presents the repo as a validation-first cache builder for
partial retrieval proofs.

![Verified retrieval cache demo](assets/screenshots/verified-cache-demo.png)

## Proof Flow

```mermaid
flowchart LR
    A[Successful full retrieval] --> B[Full-tree worker]
    B --> C[Tree builder CLI]
    C --> D[Computed root]
    D --> E{Root matches expected?}
    E -- yes --> F[Store window paths]
    E -- no --> G[Mark build error]
    F --> H[Partial retrieval verifier]
    H --> I[Range data + siblings]
    I --> J[Root validation]
```

## Sequence Diagram

```mermaid
sequenceDiagram
    participant Queue as Result Queue
    participant Worker as Full-tree Worker
    participant CLI as Merkle CLI
    participant Mongo as Window Path Store
    participant Client as Partial Read Client

    Queue->>Worker: successful retrieval record
    Worker->>CLI: stream source bytes
    CLI-->>Worker: root + window paths
    Worker->>Mongo: upsert proof artifact
    Client->>Mongo: request window path
    Mongo-->>Client: siblings
    Client-->>Client: recompute and compare root
```

## Artifact Entities

```mermaid
erDiagram
    RETRIEVAL_RESULT ||--|| TREE_BUILD : triggers
    TREE_BUILD ||--o{ WINDOW_PATH : emits
    WINDOW_PATH ||--o{ SIBLING_NODE : contains
    WINDOW_PATH ||--|| ROOT_VALIDATION : supports

    WINDOW_PATH {
      string piece
      int window_id
      int start_leaf
      int leaf_count
    }
    SIBLING_NODE {
      int level
      int index
      string hash
    }
```

## Sample Window-path Artifact

```json
{
  "piece": "baga...",
  "hash_algo": "poseidon-filecoin",
  "arity": 8,
  "window_size_bytes": 1048576,
  "window_paths": [
    {
      "window_id": 42,
      "start_leaf": 1376256,
      "leaf_count": 32768,
      "siblings": [["hex32", "..."], ["hex32", "..."]]
    }
  ],
  "validation": "root_matched"
}
```
