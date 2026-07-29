# AtlasStore — Internal Reference

> **Purpose:** How THIS project is structured — file connections, request flows, and implementation details.
> This file is your reference for understanding HOW the project is built.

---

## Directory Map

```
AtlasStore/
├── cmd/
│   ├── gateway/
│   │   └── main.go               ← Entry point: API Gateway (port 8000)
│   └── storagenode/
│       └── main.go               ← Entry point: Storage Node (gRPC, port 9001+)
│
├── internal/
│   ├── config/
│   │   └── config.go             ← Reads .env → typed *Config struct
│   ├── db/
│   │   ├── db.go                 ← Opens PostgreSQL connection pool
│   │   ├── migrate.go            ← Runs SQL migrations on startup
│   │   ├── user_repo.go          ← CreateUser, GetUserByEmail
│   │   ├── node_repo.go          ← RegisterNode, GetAllNodes, UpdateNodeStatus
│   │   └── object_repo.go        ← CreateObject, CreateChunk, GetObjectByID,
│   │                                GetChunksByObjectID, ListObjects, DeleteObject,
│   │                                GetAllChunks, AddChunkLocation, RemoveChunkLocation,
│   │                                CreateMultipartUpload, CreateMultipartChunk, CompleteMultipartUpload
│   ├── auth/
│   │   ├── handler.go            ← Register + Login HTTP handlers
│   │   └── middleware.go         ← JWT validation middleware
│   ├── api/
│   │   ├── router.go             ← All HTTP routes wired here; starts background workers
│   │   ├── storage_client.go     ← gRPC client: gateway → storage nodes (with connection caching)
│   │   ├── object_handler.go     ← Upload, Download, List, Delete, Multipart handlers
│   │   ├── node_handler.go       ← POST /nodes/register handler
│   │   ├── health_checker.go     ← Background goroutine: pings nodes via gRPC every 10s
│   │   ├── ring_manager.go       ← Background goroutine: syncs Hash Ring from PostgreSQL every 10s
│   │   └── rebalancer.go         ← Background goroutine: migrates misplaced chunks every 60s
│   ├── crypto/
│   │   └── aes.go                ← AES-GCM 256-bit Encrypt/Decrypt helpers
│   └── storage/
│       └── disk.go               ← gRPC StorageNodeServer: SaveChunk, GetChunk, DeleteChunk, Health
│
├── pkg/
│   ├── pb/
│   │   ├── storage.proto         ← Protocol Buffer definitions
│   │   ├── storage.pb.go         ← Generated: message types
│   │   └── storage_grpc.pb.go    ← Generated: gRPC server/client interfaces
│   └── ring/
│       └── ring.go               ← Consistent Hash Ring implementation (virtual nodes, thread-safe)
│
├── migrations/
│   ├── 000001_init_schema.up.sql     ← Creates users, objects, chunks tables
│   ├── 000002_storage_nodes.up.sql   ← Adds nodes, chunk_locations tables
│   ├── 000003_multipart.up.sql       ← Adds multipart_uploads, multipart_chunks tables
│   └── 000004_replication.up.sql     ← Adjusts chunk_locations for multi-node replication
│
├── data/
│   ├── nodeA/                        ← Created at runtime
│   │   └── b94d27b9...              ← Chunk files: name = sha256(plaintext), content = AES-GCM ciphertext
│   └── nodeB/
│
├── web/                             ← Vanilla HTML/JS/CSS Dashboard
│   ├── index.html
│   ├── style.css
│   └── app.js
│
├── docker-compose.yml  ← PostgreSQL on host port 5433
├── go.mod              ← Module: github.com/farhan/atlasstore
└── .env                ← Runtime config (never commit)
```

---

## Architecture: How All Components Connect

```
CLIENT (Browser / curl)
  │  HTTP :8000
  ▼
┌─────────────────────────────────────────────────────────┐
│  GATEWAY (cmd/gateway/main.go)          Control Plane   │
│                                                         │
│  ┌─────────────┐   ┌──────────────┐   ┌─────────────┐  │
│  │ Auth Handler│   │Object Handler│   │Node Handler │  │
│  │  /auth/*    │   │  /objects/*  │   │/nodes/reg.  │  │
│  └─────────────┘   └──────────────┘   └─────────────┘  │
│                            │                            │
│  ┌─────────────────────────▼──────────────────────────┐ │
│  │            StorageClient (gRPC, cached conns)       │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─────────────┐   ┌──────────────┐   ┌─────────────┐  │
│  │ RingManager │   │HealthChecker │   │ Rebalancer  │  │
│  │ (10s sync)  │   │  (10s ping)  │   │  (60s scan) │  │
│  └─────────────┘   └──────────────┘   └─────────────┘  │
│            │               │                  │         │
│            └───────────────▼──────────────────┘         │
│                      PostgreSQL                         │
└─────────────────────────────────────────────────────────┘
  │  internal gRPC :9001, :9002, ...
  ┌──────────┐  ┌──────────┐  ┌──────────┐
  │  Node A  │  │  Node B  │  │  Node C  │     Data Plane
  │  :9001   │  │  :9002   │  │  :9003   │
  │  /data/  │  │  /data/  │  │  /data/  │
  │  nodeA/  │  │  nodeB/  │  │  nodeC/  │
  └──────────┘  └──────────┘  └──────────┘
```

---

## File-by-File Breakdown

### `internal/config/config.go`
Reads `.env` → typed `*Config`. Called once in `main()`. Every other package gets `cfg` passed in — no globals.

### `internal/db/db.go`
`Connect(dsn)` → opens pool, calls `Ping()` to verify. `sql.Open()` alone does NOT connect.

### `internal/db/migrate.go`
`RunMigrations(db, path)` → applies all pending `.up.sql` files. `ErrNoChange` = already current, not an error.

### `internal/db/user_repo.go`

| Function | SQL | Notes |
|---|---|---|
| `CreateUser(db, email, hash)` | `INSERT INTO users RETURNING id` | hash = bcrypt output |
| `GetUserByEmail(email, db)` | `SELECT WHERE email=$1` | returns `nil,nil` if not found |

### `internal/db/node_repo.go`

| Function | SQL | Notes |
|---|---|---|
| `RegisterNode(db, address)` | `INSERT ... ON CONFLICT DO UPDATE` | upserts address, sets `is_active=true` |
| `GetAllNodes(db)` | `SELECT * FROM nodes` | returns all known nodes |
| `UpdateNodeStatus(db, address, isAlive)` | `UPDATE nodes SET is_active=$2` | called by health checker |

### `internal/db/object_repo.go`

| Function | SQL | Notes |
|---|---|---|
| `CreateObject(...)` | `INSERT INTO objects RETURNING id` | called once per upload |
| `CreateChunk(...)` | `INSERT INTO chunks + chunk_locations` | one row per chunk per node |
| `GetObjectByID(db, id, userID)` | `WHERE id=$1 AND user_id=$2` | ownership enforced in SQL |
| `GetChunksByObjectID(db, id)` | `GROUP BY chunk_id ORDER BY chunk_index` | returns chunks with all their node addresses |
| `GetAllChunks(db)` | same as above without `WHERE` | used by Rebalancer |
| `AddChunkLocation(db, chunkID, addr)` | `INSERT ON CONFLICT DO NOTHING` | adds a replica location |
| `RemoveChunkLocation(db, chunkID, addr)` | `DELETE WHERE chunk_id=$1 AND node_address=$2` | removes stale location |
| `ListObjects(db, userID)` | `WHERE user_id=$1 ORDER BY created_at DESC` | only user's own files |
| `DeleteObject(db, id, userID)` | `DELETE WHERE id=$1 AND user_id=$2` | CASCADE removes chunk rows |

### `pkg/ring/ring.go`

The consistent hashing ring. Uses **SHA-256** to hash node addresses and chunk keys into a `uint32` space. Each physical node gets **50 virtual nodes** for even distribution.

| Method | Description |
|---|---|
| `AddNode(address)` | Hashes 50 virtual node keys and inserts them into a sorted `[]uint32` slice |
| `RemoveNode(address)` | Filters the sorted slice, removing all virtual nodes pointing to this address |
| `GetNodes(chunkHash, count)` | Binary searches for the first clockwise virtual node ≥ hash, walks ring for `count` distinct physical nodes |
| `IsEmpty()` | Returns true if no physical nodes are in the ring |

All methods are thread-safe via an internal `sync.RWMutex`.

### `internal/api/ring_manager.go`

```
NewRingManager(db, vNodes) → RingManager{DB, Ring}
    ↓
SyncLoop() → goroutine:
    every 10s:
        db.GetAllNodes() → for each node:
            if is_active: Ring.AddNode(address)
            else:         Ring.RemoveNode(address)
```

The `RingManager` keeps the local Hash Ring in sync with PostgreSQL, so any Gateway instance will route to the same nodes.

### `internal/api/health_checker.go`

```
StartHealthChecker(db, storageClient) → goroutine:
    every 10s:
        db.GetAllNodes() → for each node (in parallel goroutines):
            storageClient.Health(node.Address) → gRPC Health()
            db.UpdateNodeStatus(address, isAlive)
```

Health checks now run in parallel goroutines per node (one goroutine per node per cycle), so a slow node doesn't block all checks.

### `internal/api/rebalancer.go`

```
StartRebalancer(ringManager, storageClient, replicationFactor) → goroutine:
    every 60s:
        db.GetAllChunks() → for each chunk:
            expectedNodes = Ring.GetNodes(chunk.Hash, replicationFactor)
            currentNodes  = chunk.NodeAddresses (from DB)

            missingNodes  = expectedNodes - currentNodes
            obsoleteNodes = currentNodes - expectedNodes

            if both empty → skip (chunk is balanced)

            if missingNodes:
                download chunk from any currentNode via gRPC
                upload to all missingNodes via gRPC
                db.AddChunkLocation() for each

            if obsoleteNodes:
                storageClient.DeleteChunk() from each
                db.RemoveChunkLocation() for each
```

### `internal/api/storage_client.go`

gRPC client with connection caching. Uses double-checked locking to avoid creating duplicate connections to the same node address.

| Method | gRPC Call | Notes |
|---|---|---|
| `SaveChunk(addresses, hash, data)` | `StorageNode.SaveChunk` | parallel goroutines per address |
| `GetChunk(address, hash)` | `StorageNode.GetChunk` | returns raw bytes (encrypted ciphertext) |
| `DeleteChunk(address, hash)` | `StorageNode.DeleteChunk` | best-effort |
| `Health(address)` | `StorageNode.Health` | 2s timeout, returns `bool` |

### `internal/api/object_handler.go`

**Upload (`POST /objects`):**
```
1. get userID from context
2. loop: io.ReadFull(r.Body, 5MB) → chunk bytes
   → errgroup goroutine:
       hash = sha256hex(plaintext chunk)
       encryptedChunk = AES-GCM Encrypt(chunk, key)
       nodeAddresses = RingManager.Ring.GetNodes(hash, replicationFactor)
       StorageClient.SaveChunk(nodeAddresses, hash, encryptedChunk) via gRPC
       append to metas
3. g.Wait() — all chunks uploaded
4. db.CreateObject → objectID
5. db.CreateChunk × N
6. return {id, name, size_bytes, content_type}
```

**Download (`GET /objects/{id}`):**
```
1. db.GetObjectByID(id, userID)  ← ownership check in SQL
2. db.GetChunksByObjectID(id)    ← ordered by chunk_index
3. for each chunk:
   → try each nodeAddress:
       StorageClient.GetChunk → encrypted bytes
       AES-GCM Decrypt(bytes, key) → plaintext
       sha256hex(plaintext) == chunk.Hash? ← integrity check
       w.Write(plaintext) → streams to client
```

**Delete (`DELETE /objects/{id}`):**
```
1. db.GetChunksByObjectID → save addresses
2. db.DeleteObject(id, userID)   ← CASCADE removes chunk_locations too
3. goroutine per node: StorageClient.DeleteChunk (best effort)
4. wg.Wait()
5. 204 No Content
```

### `internal/crypto/aes.go`

AES-GCM 256-bit encryption.

- **Encrypt:** Generates a random 12-byte nonce, seals ciphertext with nonce prepended: `[nonce (12B) | ciphertext | auth_tag (16B)]`
- **Decrypt:** Splits nonce from ciphertext, opens and authenticates

The filename on disk is always `sha256(plaintext)` — so the hash is computed *before* encryption. The encrypted bytes live inside the file.

### `internal/storage/disk.go`

gRPC `StorageNodeServer` implementation. All chunk files are stored as:
```
{DataDir}/{sha256-hash}   ← name = plaintext hash, content = encrypted bytes
```

| RPC | Operation |
|---|---|
| `SaveChunk(hash, data)` | `os.MkdirAll` + `os.Create` + `file.Write(req.Data)` |
| `GetChunk(hash)` | `os.ReadFile` |
| `DeleteChunk(hash)` | `os.Remove` |
| `Health()` | returns `{status: "ok", dataDir: ...}` |

---

## Complete Request Flows

### Upload Flow (Phase 6 — with Ring + Encryption)
```
POST /objects (Bearer token + raw file bytes)
  ↓
RequireAuth → JWT validate → inject userID into context
  ↓
objectHandler.Upload()
  ├── io.ReadFull(body, 5MB) × N chunks
  │   └── (parallel errgroup goroutine per chunk):
  │       hash = sha256(plaintext chunk)
  │       encrypted = AES-GCM-Encrypt(chunk, key)
  │       nodes = Ring.GetNodes(hash, replicationFactor) → e.g. ["localhost:9001", "localhost:9002"]
  │       gRPC SaveChunk(nodes, hash, encrypted) → parallel per node
  │       → disk.go: os.Create(./data/nodeA/{hash}), file.Write(encrypted bytes)
  ├── db.CreateObject → INSERT INTO objects
  └── db.CreateChunk + chunk_locations × N
  ↓
{"id": "uuid", "name": "file.pdf", ...}
```

### Rebalancing Flow (Phase 6.3)
```
[Every 60 seconds, background goroutine]
  ↓
db.GetAllChunks() → all chunks + their current node locations
  ↓
for each chunk:
  expectedNodes = Ring.GetNodes(chunk.Hash, RF)   ← where it SHOULD be
  currentNodes  = chunk.NodeAddresses             ← where it IS

  missingNodes  = expectedNodes - currentNodes
  obsoleteNodes = currentNodes  - expectedNodes

  if missingNodes:
    chunkData = gRPC GetChunk(anyCurrentNode, hash)  ← download encrypted bytes
    gRPC SaveChunk(missingNodes, hash, chunkData)    ← upload to new nodes
    db.AddChunkLocation(chunkID, newNode) × missing

  if obsoleteNodes:
    gRPC DeleteChunk(obsoleteNode, hash) × obsolete
    db.RemoveChunkLocation(chunkID, oldNode) × obsolete
```

### Download Flow
```
GET /objects/{id} (Bearer token)
  ↓
objectHandler.Download()
  ├── db.GetObjectByID(id, userID) → WHERE id=$1 AND user_id=$2
  ├── db.GetChunksByObjectID(id) → ORDER BY chunk_index ASC
  └── for each chunk:
      → for each nodeAddress (replica fallback):
          gRPC GetChunk(node, hash) → encrypted bytes
          AES-GCM Decrypt(bytes, key) → plaintext
          sha256hex(plaintext) == chunk.Hash? ← data integrity check
          w.Write(plaintext) → stream to client
```

### Delete Flow
```
DELETE /objects/{id} (Bearer token)
  ↓
objectHandler.Delete()
  ├── db.GetChunksByObjectID → get all replica addresses
  ├── db.DeleteObject(id, userID) → DELETE + CASCADE removes chunk_locations
  └── (parallel goroutines) gRPC DeleteChunk per replica node
  ↓
204 No Content
```

---

## Background Workers Summary

| Worker | File | Interval | What It Does |
|---|---|---|---|
| `RingManager.SyncLoop` | `ring_manager.go` | 10s | Fetches active nodes from DB, rebuilds Hash Ring |
| `StartHealthChecker` | `health_checker.go` | 10s | Pings each node via gRPC, updates `is_active` in DB |
| `StartRebalancer` | `rebalancer.go` | 60s | Finds misplaced chunks, migrates them via gRPC, updates DB |

All three run as goroutines started in `router.go`'s `NewRouter()` function.

---

## What Is NOT Built Yet

| Feature | Phase | Notes |
|---|---|---|
| Raft / Consensus | Phase 7 | Nodes elect leader, manage cluster without relying solely on PostgreSQL |
| Data Compression | Phase 8 | Compress chunks before encrypting (snappy/zstd) |
| Prometheus Metrics | Phase 11 | Expose `/metrics` endpoint for Grafana dashboards |
| Circuit Breaker | Phase 8 | Stop hammering a dead node, fail fast |
