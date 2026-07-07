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
│       └── main.go               ← Entry point: Storage Node (port 9000)
│
├── internal/
│   ├── config/
│   │   └── config.go             ← Reads .env → typed *Config struct
│   ├── db/
│   │   ├── db.go                 ← Opens PostgreSQL connection pool
│   │   ├── migrate.go            ← Runs SQL migrations on startup
│   │   ├── user_repo.go          ← CreateUser, GetUserByEmail
│   │   └── object_repo.go        ← CreateObject, CreateChunk, GetObjectByID,
│   │                                GetChunksByObjectID, ListObjects, DeleteObject
│   ├── auth/
│   │   ├── handler.go            ← Register + Login HTTP handlers
│   │   └── middleware.go         ← JWT validation middleware
│   ├── api/
│   │   ├── router.go             ← All HTTP routes wired here
│   │   ├── storage_client.go     ← HTTP client: gateway → storage node
│   │   └── object_handler.go     ← Upload, Download, List, Delete handlers
│   └── storage/
│       └── disk.go               ← Chunk save/read/delete on local disk
│
├── migrations/
│   ├── 000001_init_schema.up.sql    ← Creates users, objects, chunks tables
│   └── 000001_init_schema.down.sql ← Drops tables (rollback)
│
├── data/
│   └── node1/                       ← Created at runtime
│       └── 4dcfa58d...              ← Chunk files named by SHA-256 hash
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

## Two-Server Architecture

```
CLIENT
  │  HTTP :8000
  ▼
┌─────────────────────────────────────────┐
│  GATEWAY (cmd/gateway/main.go)          │  ← "Brain" / Control Plane
│  - Auth (JWT register/login)            │
│  - Owns PostgreSQL (users/objects/chunks│
│  - Chunks files, orchestrates storage   │
│  - Talks to storage nodes via HTTP      │
└─────────────────────────────────────────┘
  │  internal HTTP calls :9000
  ▼
┌─────────────────────────────────────────┐
│  STORAGE NODE (cmd/storagenode/main.go) │  ← "Muscle" / Data Plane
│  - No auth, no database                 │
│  - Saves/reads/deletes chunk files      │
│  - Files named by their SHA-256 hash    │
└─────────────────────────────────────────┘
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

### `internal/db/object_repo.go`

| Function | SQL | Notes |
|---|---|---|
| `CreateObject(db, userID, name, ct, size)` | `INSERT INTO objects RETURNING id` | called once per upload |
| `CreateChunk(db, objectID, idx, hash, size, addr)` | `INSERT INTO chunks` | called once per chunk |
| `GetObjectByID(db, objectID, userID)` | `WHERE id=$1 AND user_id=$2` | ownership enforced in SQL |
| `GetChunksByObjectID(db, objectID)` | `ORDER BY chunk_index ASC` | order = reassembly order |
| `ListObjects(db, userID)` | `WHERE user_id=$1 ORDER BY created_at DESC` | only user's own files |
| `DeleteObject(db, objectID, userID)` | `DELETE WHERE id=$1 AND user_id=$2` | checks `RowsAffected` |

### `internal/auth/handler.go`

**Register:** decode → check duplicate → bcrypt hash → insert → sign JWT → 201

**Login:** decode → fetch user → `bcrypt.CompareHashAndPassword` → sign JWT → 200

Both wrong-email and wrong-password return `"invalid credentials"` — never reveal which.

### `internal/auth/middleware.go`

```
Authorization: Bearer <token>
    ↓
jwt.Parse(token, verify HMAC signature)
    ↓
extract claims["sub"] = userID
    ↓
context.WithValue(r.Context(), UserIDKey, userID)
    ↓
next.ServeHTTP(w, r.WithContext(ctx))
```

Downstream handlers read: `r.Context().Value(auth.UserIDKey).(string)`

### `internal/api/storage_client.go`

HTTP client the gateway uses to call the storage node. Wraps `http.Client` (which has connection pooling built in).

| Method | Calls | Notes |
|---|---|---|
| `SaveChunk(hash, data)` | `POST /chunk` with `X-Chunk-Hash` header | sends raw bytes |
| `GetChunk(hash)` | `GET /chunk/{hash}` | returns raw bytes |
| `DeleteChunk(hash)` | `DELETE /chunk/{hash}` | best-effort |

### `internal/api/object_handler.go`

**Upload (`POST /objects`):**
```
1. get userID from context
2. get filename from X-Filename header
3. loop: io.ReadFull(r.Body, 5MB buf)
   → sha256hex(chunk)
   → StorageClient.SaveChunk(hash, chunk)
   → append to metas list
4. ← loop done
5. db.CreateObject → get objectID
6. db.CreateChunk × N (one per chunk)
7. return {"id": objectID, ...}
```

**Download (`GET /objects/{id}`):**
```
1. db.GetObjectByID(id, userID)  ← ownership check in SQL
2. db.GetChunksByObjectID(id)    ← ordered by chunk_index
3. for each chunk:
   StorageClient.GetChunk(hash)  → bytes
   w.Write(bytes)                → streams to client
```

**List (`GET /objects`):** `db.ListObjects(userID)` → JSON array

**Delete (`DELETE /objects/{id}`):**
```
1. db.GetChunksByObjectID → save addresses
2. db.DeleteObject(id, userID)   ← ownership check + CASCADE cleans chunks table
3. StorageClient.DeleteChunk × N ← best effort
4. 204 No Content
```

### `internal/storage/disk.go`

Handles chunk HTTP endpoints on the storage node. Files are stored as:
```
{DataDir}/{sha256-hash}   ← filename = hash, content = raw bytes
```

| Method | Route | Operation |
|---|---|---|
| `SaveChunk` | `POST /chunk` | `os.Create` + `io.Copy(file, r.Body)` |
| `GetChunk` | `GET /chunk/{hash}` | `os.Open` + `io.Copy(w, file)` |
| `DeleteChunk` | `DELETE /chunk/{hash}` | `os.Remove` |
| `Health` | `GET /health` | returns `{"status":"ok"}` |

---

## Complete Request Flows

### Upload Flow
```
curl POST /objects (Bearer token + file bytes)
  ↓
RequireAuth middleware → validates JWT → injects userID
  ↓
objectHandler.Upload()
  ├── io.ReadFull(body, 5MB) × N chunks
  │     sha256hex(chunk) → hash
  │     StorageClient.SaveChunk(hash, chunk)
  │       → POST http://localhost:9000/chunk
  │       → disk.go: os.Create(./data/node1/{hash})
  │                  io.Copy(file, body)
  ├── db.CreateObject → INSERT INTO objects
  └── db.CreateChunk × N → INSERT INTO chunks
  ↓
{"id":"uuid","name":"hello.txt","size_bytes":17}
```

### Download Flow
```
curl GET /objects/{id} (Bearer token)
  ↓
RequireAuth → injects userID
  ↓
objectHandler.Download()
  ├── db.GetObjectByID(id, userID) → WHERE id=$1 AND user_id=$2
  ├── db.GetChunksByObjectID → ORDER BY chunk_index
  └── for chunk in chunks:
        StorageClient.GetChunk(chunk.Hash)
          → GET http://localhost:9000/chunk/{hash}
          → disk.go: os.Open(./data/node1/{hash})
                     io.Copy(w, file)
        w.Write(bytes) → streams to client
  ↓
"Hello AtlasStore!"
```

### Delete Flow
```
curl DELETE /objects/{id} (Bearer token)
  ↓
objectHandler.Delete()
  ├── db.GetChunksByObjectID → get node addresses
  ├── db.DeleteObject(id, userID) → DELETE WHERE id=$1 AND user_id=$2
  │     CASCADE → chunks rows auto-deleted
  └── StorageClient.DeleteChunk × N (best effort)
        → DELETE http://localhost:9000/chunk/{hash}
        → disk.go: os.Remove(./data/node1/{hash})
  ↓
204 No Content
```

---

## What Is NOT Built Yet

| Component | Location | Needed For |
|---|---|---|
| SHA-256 integrity check on download | object_handler | Phase 2 — re-hash chunk, compare |
| Parallel chunk I/O | object_handler | Phase 2 — goroutines |
| Health-check polling | gateway | Phase 3 — detect dead nodes |
| Node registration | storagenode | Phase 3 — nodes announce themselves |
| Replication (N copies per chunk) | gateway | Phase 4 |
| gRPC | internal comms | Phase 5 |
