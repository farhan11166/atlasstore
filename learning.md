# AtlasStore — Learning Reference

> **Purpose:** Concepts, patterns, and technologies explained from first principles.
> This file is your reference for understanding WHY things work the way they do.

---

## Table of Contents

**Week 1**
1. [Why Go?](#1-why-go)
2. [Go Module System](#2-go-module-system)
3. [Project Layout — `internal/` and `cmd/`](#3-project-layout)
4. [Configuration Management](#4-configuration-management)
5. [PostgreSQL Fundamentals](#5-postgresql-fundamentals)
6. [Database Migrations](#6-database-migrations)
7. [Connection Pooling](#7-connection-pooling)
8. [UUIDs as Primary Keys](#8-uuids-as-primary-keys)
9. [Foreign Keys and Cascading Deletes](#9-foreign-keys-and-cascading-deletes)
10. [What is a DSN?](#10-what-is-a-dsn)
11. [Error Wrapping in Go](#11-error-wrapping-in-go)
12. [Go Pointers — When and Why](#12-go-pointers--when-and-why)
13. [Blank Imports](#13-blank-imports)

**Week 2**
14. [Password Hashing with bcrypt](#14-password-hashing-with-bcrypt)
15. [JWT — JSON Web Tokens](#15-jwt--json-web-tokens)
16. [HTTP Middleware Pattern](#16-http-middleware-pattern)
17. [Go Context — Passing Data Through a Request](#17-go-context)
18. [Struct-Based Handlers (Dependency Injection)](#18-struct-based-handlers)
19. [Two-Server Architecture](#19-two-server-architecture)
20. [Why `io.Copy` Instead of `ReadAll`](#20-why-iocopy)

**Week 3**
21. [SHA-256 — Content Hashing](#21-sha-256--content-hashing)
22. [io.ReadFull — Reading Fixed-Size Chunks](#22-ioreadfull)
23. [Streaming HTTP Responses](#23-streaming-http-responses)
24. [Ownership Enforcement in SQL vs Code](#24-ownership-enforcement-in-sql-vs-code)
25. [Best-Effort Operations](#25-best-effort-operations)

**Object Storage Concepts**
26. [Object Storage — What Problem It Solves](#26-object-storage)
27. [Chunking — Why Files Are Split](#27-chunking)

**Week 4 (Frontend)**
28. [Serving Static Files in Go](#28-serving-static-files-in-go)
29. [XMLHttpRequest vs Fetch for Uploads](#29-xmlhttprequest-vs-fetch-for-uploads)
30. [Downloading via Object URLs](#30-downloading-via-object-urls)

---

## 1. Why Go?

Go was designed at Google for **network services and infrastructure**.

| Property | What It Means |
|---|---|
| **Compiled** | Single binary — easy to ship |
| **Goroutines** | Lightweight threads — handle thousands of connections cheaply |
| **Standard library** | Built-in HTTP, crypto, file I/O — no framework needed |
| **Static typing** | Compiler catches bugs before runtime |

Why MinIO, Docker, and Kubernetes are all written in Go.

---

## 2. Go Module System

```
module github.com/farhan/atlasstore   ← unique import path prefix
go 1.24.0
require (...)                          ← locked dependencies
```

`go.sum` = cryptographic lock file. If a dependency is tampered with, `go.sum` fails. Never edit manually.

---

## 3. Project Layout

**`cmd/`** — executable entry points (one `main.go` per binary)
**`internal/`** — Go compiler enforces that ONLY this module can import these packages
**`pkg/`** — code you'd eventually share as a library

---

## 4. Configuration Management

`.env` + `godotenv` = development convenience. `godotenv.Load()` calls `os.Setenv()` for each key.

**Why a typed `Config` struct:** Raw env vars are all strings. Parsing once at startup gives typed values everywhere and fails fast on bad config — not silently mid-request.

**Why `.env` is in `.gitignore`:** Contains real secrets. In production, set env vars directly on the server.

---

## 5. PostgreSQL Fundamentals

`database/sql` = Go standard interface. `lib/pq` = PostgreSQL-specific driver that implements it. You write against `database/sql` — driver is swappable.

**`sql.Open()` does NOT connect.** It validates the driver name. `db.Ping()` is what actually tests the connection.

---

## 6. Database Migrations

Numbered SQL files tracked in a `schema_migrations` table. `golang-migrate` applies only unapplied files on startup:

```
000001 ✓ (already applied)
000002 → applies automatically
```

**`.up` / `.down` pair:** Every migration has a reverse. Drop order must be reverse of create order — FK constraints prevent dropping a table that others reference.

---

## 7. Connection Pooling

`database/sql` maintains pre-opened connections and reuses them:
- `SetMaxOpenConns(25)` — max 25 simultaneous connections
- `SetMaxIdleConns(10)` — keep 10 warm when idle

Opening a new DB connection = TCP handshake + auth + SSL. Pooling amortizes that cost.

---

## 8. UUIDs as Primary Keys

**Why not integers (1, 2, 3...)?**
1. Predictable — attacker tries ID 41, 42, 43
2. Distributed collision — two nodes might both generate `42`
3. Sequential scraping — can iterate your entire dataset

`gen_random_uuid()` from `pgcrypto` generates a 128-bit random value per row automatically.

---

## 9. Foreign Keys and Cascading Deletes

```sql
user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
```

**FK:** `objects.user_id` must be a valid `users.id`. DB enforces this — your code doesn't have to.

**CASCADE:** Delete user → auto-delete all their objects → auto-delete all their chunks. One SQL statement, entire tree cleaned up.

---

## 10. What is a DSN?

Data Source Name — the connection string:
```
host=localhost port=5433 user=atlasstore password=atlaspassword dbname=atlasstore sslmode=disable
```

Assembled once in `config.go`, stored as `cfg.DBDSN`, used everywhere else.

---

## 11. Error Wrapping in Go

```go
return nil, fmt.Errorf("could not connect: %w", err)
```

`%w` wraps the original error with context. Creates a readable chain:
```
"migrations failed: could not create driver: connection refused"
```

`errors.Is(err, migrate.ErrNoChange)` unwraps the chain to check for specific types.

---

## 12. Go Pointers — When and Why

```go
func Load() (*Config, error)  // pointer return
```

1. **Efficiency** — one copy, multiple references regardless of struct size
2. **Nilability** — `return nil, err` is idiomatic for failure
3. **`*sql.DB` must be shared** — one pool across all goroutines, never copied

---

## 13. Blank Imports

```go
import _ "github.com/lib/pq"
```

Imports for side effects only — runs the package's `init()` which registers the driver with `database/sql`. Required before `sql.Open("postgres", ...)` works.

---

## 14. Password Hashing with bcrypt

**Never store plaintext passwords.**

```go
hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
err := bcrypt.CompareHashAndPassword(hash, []byte("secret"))
```

**Why bcrypt over SHA-256?**
- bcrypt is intentionally **slow** (cost factor) — brute force takes years
- bcrypt includes a **salt** — same password hashes differently each time, defeats rainbow tables
- SHA-256 is fast and deterministic — terrible for passwords, great for data integrity

---

## 15. JWT — JSON Web Tokens

**Problem:** HTTP is stateless. After login, how does the server know who you are?

**JWT (stateless):** Server signs a token with a secret. Client stores it, sends it back with every request. Server validates the signature — no database lookup needed.

**Structure:** `HEADER.PAYLOAD.SIGNATURE` — all base64 encoded.

**Claims used:**
```json
{"sub": "user-uuid", "exp": 1783373248, "iat": 1783114048}
```

**Verification:** Re-sign header+payload with `JWT_SECRET`, compare to signature. Tampering = signature mismatch = rejected.

---

## 16. HTTP Middleware Pattern

Middleware wraps a handler — runs code before (and optionally after) the real handler:

```go
func RequireAuth(secret string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // validate token
            if invalid { http.Error(w, "401", 401); return }
            // call real handler
            next.ServeHTTP(w, r)
        })
    }
}
```

Wired in router:
```go
mux.Handle("POST /objects", protected(http.HandlerFunc(objectHandler.Upload)))
```

Request → `RequireAuth` → if valid → `Upload`. The handler knows nothing about auth.

---

## 17. Go Context

**Problem:** Middleware extracts `userID` from JWT. How does it pass that to the handler?

Can't use function params (fixed HTTP signatures). Can't use globals (concurrent requests overwrite each other).

**Solution:** `context.Context` — request-scoped key-value store:

```go
// middleware — write
ctx := context.WithValue(r.Context(), UserIDKey, userID)
next.ServeHTTP(w, r.WithContext(ctx))

// handler — read
userID := r.Context().Value(auth.UserIDKey).(string)
```

**Why a private `contextKey` type?** Prevents other packages from accidentally reading/overwriting the same key.

---

## 18. Struct-Based Handlers

**Problem:** Handlers need `*sql.DB`, `JWTSecret`, etc. without globals.

```go
type ObjectHandler struct {
    DB            *sql.DB
    StorageClient *StorageClient
    ChunkSizeMB   int
}
```

`main.go` creates the handler with explicit dependencies. This is **dependency injection** — dependencies are visible, testable, no hidden state.

---

## 19. Two-Server Architecture

**Gateway (Control Plane):** auth, database, orchestration — exposed to clients
**Storage Node (Data Plane):** disk I/O only — internal, not exposed to internet

**Why separate?**
1. Scale independently — add disk (nodes) vs add CPU (gateway) separately
2. Different hardware profiles
3. Fault isolation — gateway crash ≠ data loss
4. Enables replication — same chunk → multiple nodes (Phase 4)

---

## 20. Why `io.Copy`

```go
// BAD — 100 concurrent 5MB uploads = 500MB RAM
data, _ := io.ReadAll(r.Body)
os.WriteFile(path, data, 0644)

// GOOD — fixed ~32KB buffer regardless of file size
io.Copy(file, r.Body)
```

`io.Copy` streams bytes directly from source to destination using an internal buffer. The whole file is never in memory at once.

---

## 21. SHA-256 — Content Hashing

```go
h := sha256.Sum256(data)
hash := hex.EncodeToString(h[:])  // 64 hex characters
```

**Properties:**
- **Deterministic** — same content always = same hash
- **Fixed size** — 64 hex chars regardless of input size
- **One-way** — can't reverse the hash to get the content
- **Collision-resistant** — two different inputs will not produce the same hash

**How AtlasStore uses it:**
- The hash IS the filename of the chunk on disk
- No lookup table needed: `GET /chunk/{hash}` → open file named `{hash}`
- Phase 2 will re-hash on download and compare → integrity verification

**Why SHA-256 here but bcrypt for passwords?**
- SHA-256 is fast — good for data where you need performance
- bcrypt is slow — good for passwords where you want brute-force resistance
- Different tools for different problems

---

## 22. `io.ReadFull`

```go
n, err := io.ReadFull(r.Body, buf)
```

Reads **exactly** `len(buf)` bytes from the reader into `buf`.

| Return value | Meaning |
|---|---|
| `n == len(buf), err == nil` | Full buffer read — more data may remain |
| `n < len(buf), err == io.ErrUnexpectedEOF` | Hit EOF before filling buffer — this is the **last chunk** |
| `n == 0, err == io.EOF` | Nothing read, stream ended |

This is why the loop uses:
```go
if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
    break  // last chunk processed, exit loop
}
```

The last chunk of a file is almost always smaller than `ChunkSizeMB` — `io.ReadFull` handles this gracefully.

---

## 23. Streaming HTTP Responses

**Download doesn't load the whole file into RAM:**

```go
for _, chunk := range chunks {
    data, _ := StorageClient.GetChunk(chunk.Hash)  // fetches one chunk
    w.Write(data)                                   // writes it immediately
}
```

Each chunk is fetched and written to the response one at a time. The client receives bytes while the gateway is still fetching later chunks. This is **streaming** — essential for large files.

Without streaming:
```go
// BAD — load entire file into memory first
allBytes := fetchAllChunks()  // 10GB in RAM
w.Write(allBytes)             // then send
```

---

## 24. Ownership Enforcement in SQL vs Code

Two ways to enforce "users can only access their own data":

**Option A — Check in code:**
```go
obj, _ := db.GetObjectByID(db, objectID)  // gets any object
if obj.UserID != userID { return 403 }    // check after
```

**Option B — Enforce in SQL (what AtlasStore does):**
```sql
SELECT * FROM objects WHERE id = $1 AND user_id = $2
```

**Why SQL is better:**
- No data crosses the network if it doesn't belong to the user
- Impossible to accidentally forget the check — it's structural
- Single round trip instead of two

**Why return 404 instead of 403:**
- 403 tells the attacker the object EXISTS
- 404 reveals nothing — standard practice for private resource APIs (S3 does this)

---

## 25. Best-Effort Operations

```go
// Delete from storage node — best effort, don't fail if node is down
for _, chunk := range chunks {
    _ = h.StorageClient.DeleteChunk(chunk.Hash)
}
w.WriteHeader(http.StatusNoContent)
```

The `_` ignores the error from `DeleteChunk`. Why?

The source of truth is **PostgreSQL**. Once the DB row is deleted, the object is logically gone — users can't access it anymore. The chunk on disk becomes **orphaned** (unreachable). Whether or not the storage node successfully deletes the file is a secondary concern.

This is a common distributed systems trade-off: **don't let a storage node failure prevent a logically valid delete**. In production you'd have a background garbage collector that cleans up orphaned chunks.

---

## 26. Object Storage

A regular filesystem doesn't scale — one disk fills up, one machine goes down = data lost.

Object storage treats every file as a flat **object** (key + bytes + metadata). No folder hierarchy at storage level. Any node can store any object — trivially distributable.

AtlasStore:
```
Object = objects table row + N chunk files on disk
Key    = objects.id (UUID)
Data   = chunk files in data/node1/{hash}
Meta   = name, size_bytes, content_type
```

---

## 27. Chunking

**Without chunking:** 10GB file on one node → node fills up, no replication possible.

**With chunking (5MB pieces):**
1. **Distribution** — chunks spread across nodes
2. **Replication** — same chunk stored on N nodes (Phase 4)
3. **Parallel I/O** — fetch chunk 0 and chunk 1 simultaneously (Phase 2)
4. **Integrity** — SHA-256 hash each chunk, re-verify on download (Phase 2)

**Reassembly:** `chunks.chunk_index` determines order. Always `ORDER BY chunk_index` → concatenate → original file.

---

## 28. Serving Static Files in Go

```go
mux.Handle("/", http.FileServer(http.Dir("./web")))
```
Go's built-in file server makes hosting SPAs (Single Page Applications) incredibly easy. 
- It automatically maps URL paths to files in the `./web` directory.
- It automatically sets the correct `Content-Type` headers (`text/html`, `text/css`, `application/javascript`) based on file extensions.
- In `ServeMux`, longer matching routes take precedence. `/auth/login` is handled by the API, while a request to `/` falls back to the FileServer.

---

## 29. XMLHttpRequest vs Fetch for Uploads

You might have noticed we used `fetch()` for Login and Delete, but `XMLHttpRequest` (XHR) for Uploads. Why?

**The problem with `fetch()`:** It does not currently support *upload progress events*. You only know when the upload is 0% and 100%.

**The XHR solution:**
```js
xhr.upload.onprogress = (e) => {
    const pct = Math.round((e.loaded / e.total) * 100)
    updateProgressBar(pct)
}
```
XHR natively emits progress events as chunks of the TCP payload are acknowledged by the server. This allows us to build a smooth, real-time progress bar.

---

## 30. Downloading via Object URLs

How do you force a file download via an API secured by a JWT? You can't just use `<a href="/objects/123">` because you can't attach an `Authorization` header to standard HTML links.

**The Object URL Pattern:**
1. Fetch the file via JS using the token.
2. Convert the response into a binary `Blob` (Browser memory).
3. Create a temporary internal browser URL pointing to that Blob.
4. Create an invisible `<a>` tag, click it programmatically, and clean up.

```js
const blob = await res.blob()
const url = URL.createObjectURL(blob) // looks like: blob:http://localhost:8000/1234-5678
const a = document.createElement('a')
a.href = url
a.download = "filename.txt"
a.click() 
URL.revokeObjectURL(url) // free memory
```

---

## Concepts Still To Come

| Concept | Phase | Why |
|---|---|---|
| **Health checks** | Phase 3 | Gateway polls `/health` on each node, skips dead ones |
| **Node registration** | Phase 3 | Nodes announce themselves to gateway on startup |
| **Replication** | Phase 4 | Write each chunk to N nodes, read from any |
| **Consistent Hashing** | Phase 6 | Distribute chunks across nodes without a lookup table |
| **Raft Consensus** | Phase 7 | How distributed nodes agree on cluster state |
| **gRPC** | Phase 5 | Binary protocol replacing HTTP for internal comms |
| **Encryption at rest** | Phase 8 | AES-256 chunks before writing to disk |
| **Prometheus metrics** | Phase 11 | Measuring upload latency, node health, throughput |

---

## 31. Data Integrity & Checksums (Phase 2)
When downloading, we re-hash the data received from the storage node and compare it to the hash stored in the database. If they mismatch, it prevents silent data corruption (e.g., bit rot on the hard drive) or network tampering.

## 32. Parallel I/O with Goroutines (Phase 2)
Instead of waiting for one chunk to delete or upload before starting the next, we use `sync.WaitGroup` and `errgroup` to launch multiple background workers (`goroutines`).
- `WaitGroup` handles fire-and-forget concurrency (e.g., deleting chunks where we ignore errors).
- `errgroup` handles concurrency where we care if one of the background tasks fails (e.g., uploading chunks where a failure means the upload must abort).

## 33. SQL Transactions (Phase 2)
`CompleteMultipartUpload` requires moving data from temp tables to permanent tables, and then deleting the temp tables. If the server crashes in the middle, the database could be left in an inconsistent state.
`tx, err := db.Begin()` starts a transaction. All queries use `tx`. If anything fails, `tx.Rollback()` safely undoes everything. Only `tx.Commit()` makes the changes permanent.

## 34. Multipart Uploads (Phase 2)
Uploading a 10GB file in a single HTTP request is fragile. If the connection drops at 99%, the user has to restart from 0%. Multipart Uploads solve this by slicing the file on the client side (`file.slice()`) and uploading the 5MB chunks independently in parallel. The backend stores metadata in temporary tables until the client sends a `complete` signal, which stitches them together.
