# AtlasStore — Learning Reference

> **Purpose:** Concepts, patterns, and technologies explained from first principles.
> This file is your reference for understanding WHY things work the way they do.

---

## Table of Contents

**Week 1 Concepts**
1. [Why Go?](#1-why-go)
2. [Go Module System](#2-go-module-system)
3. [Project Layout — Why `internal/` and `cmd/`?](#3-project-layout)
4. [Configuration Management](#4-configuration-management)
5. [Database — PostgreSQL Fundamentals](#5-database--postgresql-fundamentals)
6. [Database Migrations — Why They Exist](#6-database-migrations)
7. [Connection Pooling](#7-connection-pooling)
8. [UUIDs as Primary Keys](#8-uuids-as-primary-keys)
9. [Foreign Keys and Cascading Deletes](#9-foreign-keys-and-cascading-deletes)
10. [What is a DSN?](#10-what-is-a-dsn)
11. [Error Wrapping in Go](#11-error-wrapping-in-go)
12. [Go Pointers — When and Why](#12-go-pointers--when-and-why)
13. [Blank Imports (`_ "package"`)](#13-blank-imports)

**Week 2 Concepts**
14. [Password Hashing with bcrypt](#14-password-hashing-with-bcrypt)
15. [JWT — JSON Web Tokens](#15-jwt--json-web-tokens)
16. [HTTP Middleware Pattern](#16-http-middleware-pattern)
17. [Go Context — Passing Data Through a Request](#17-go-context--passing-data-through-a-request)
18. [Struct-Based Handlers (Dependency Injection)](#18-struct-based-handlers-dependency-injection)
19. [Two-Server Architecture — Gateway vs Storage Node](#19-two-server-architecture)
20. [Why `io.Copy` Instead of `ReadAll`](#20-why-iocopy-instead-of-readall)

**Object Storage Concepts**
21. [Object Storage — What Problem It Solves](#21-object-storage--what-problem-it-solves)
22. [Chunking — Why Files Are Split](#22-chunking--why-files-are-split)

---

## 1. Why Go?

Go was designed at Google for **network services and infrastructure**. That makes it a perfect fit for AtlasStore.

| Property | What It Means For You |
|---|---|
| **Compiled** | Single binary — easy to ship to servers |
| **Goroutines** | Lightweight threads — handle thousands of simultaneous uploads cheaply |
| **Standard library** | Built-in HTTP server, crypto, file I/O — no framework needed |
| **Static typing** | Compiler catches bugs before runtime |
| **Fast compilation** | Feedback loop is seconds, not minutes |

Python would be simpler but would struggle at the concurrency scale that storage systems require. Rust would be faster but far harder to write. Go is why MinIO, Docker, and Kubernetes are all written in it.

---

## 2. Go Module System

```
module github.com/farhan/atlasstore   ← import path prefix for YOUR code
go 1.24.0                             ← minimum Go version
require (...)                         ← external dependencies with versions
```

**Why the import path looks like a GitHub URL:** Go uses URLs as globally unique package names. It doesn't mean your code must be on GitHub — it's a naming convention that guarantees no two packages in the world collide.

**`go.sum`:** A cryptographic lock file. Every dependency has a hash. If someone tampers with a library, `go.sum` fails. You never edit it manually.

---

## 3. Project Layout

### Why `cmd/`?
Holds **executable entry points** — programs you can run.
```
cmd/gateway/main.go      → builds the API Gateway binary
cmd/storagenode/main.go  → builds the Storage Node binary
```
One repository, two runnable programs.

### Why `internal/`?
A **Go compiler enforcement rule** — any package inside `internal/` can ONLY be imported by code in the parent module. External modules cannot import your internal packages, enforcing that implementation details stay private.

### Why `pkg/`?
For code you'd eventually share as a standalone library. Opposite of `internal/`.

---

## 4. Configuration Management

**The problem:** You need passwords and secrets at runtime without hardcoding them.

**Solution:** Environment variables. The `.env` file + `godotenv` is a development convenience:
```
godotenv.Load() → reads KEY=VALUE from .env → calls os.Setenv() for each
```

**Why `.env` is in `.gitignore`:** It contains real secrets. In production, env vars are set directly on the server or via a secrets manager — never committed to git.

**Why a typed `Config` struct:** Raw env vars are all strings. Parsing once at startup gives you typed values everywhere and fails fast if anything is missing or malformed.

---

## 5. Database — PostgreSQL Fundamentals

Data is stored in **tables** connected by **foreign keys**. SQL is how you talk to it.

**`database/sql` vs `lib/pq`:**
```
database/sql    ← Go standard library interface (generic)
lib/pq          ← PostgreSQL-specific driver that implements it
```
You write code against `database/sql` — if you switch drivers later, only one import changes.

---

## 6. Database Migrations

**The problem:** Your schema evolves over time. How do you track which changes have been applied to which database?

**Solution:** Numbered SQL files. `golang-migrate` tracks applied files in a `schema_migrations` table:
```
000001_init_schema.up.sql      ← applied ✓
000002_add_role.up.sql         ← pending → applies automatically on next startup
```

**`.up` / `.down` pair:** Every migration has a reverse. Drop order must be reverse of create order (FK constraints):
```sql
DROP TABLE chunks;   ← depends on objects, drop first
DROP TABLE objects;  ← depends on users, drop second
DROP TABLE users;    ← drop last
```

---

## 7. Connection Pooling

Opening a new DB connection is slow (TCP + auth + SSL). `database/sql` maintains a **pool** of pre-opened connections:

```
Request 1 → borrows connection → runs query → returns connection to pool
Request 2 → borrows same connection (reused)
```

- `SetMaxOpenConns(25)` — max 25 simultaneous connections
- `SetMaxIdleConns(10)` — keep 10 alive when idle, ready immediately

**`sql.Open()` does NOT connect.** It just validates the driver name. `db.Ping()` is what actually tests the connection.

---

## 8. UUIDs as Primary Keys

**Why not auto-increment (1, 2, 3...)?**
1. **Predictable** — attacker can try IDs 41, 42, 43
2. **Distributed collision** — two nodes might both generate `42`
3. **Sequential scraping** — can iterate your entire dataset

**UUID:** 128-bit cryptographically random value. `gen_random_uuid()` from `pgcrypto` generates one for every new row automatically. Looks like: `550e8400-e29b-41d4-a716-446655440000`

---

## 9. Foreign Keys and Cascading Deletes

```sql
user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
```

**Foreign key:** `objects.user_id` must be a valid `users.id`. Database enforces this — your code doesn't have to.

**CASCADE:** Delete a user → automatically delete all their objects → automatically delete all chunks. One SQL statement, entire tree cleaned up safely.

---

## 10. What is a DSN?

DSN = **Data Source Name**. The connection string for a database driver.

```
host=localhost port=5433 user=atlasstore password=atlaspassword dbname=atlasstore sslmode=disable
```

Assembled once in `config.go` and stored as `cfg.DBDSN`. Everything else just uses `cfg.DBDSN`.

---

## 11. Error Wrapping in Go

Go has no exceptions. Errors are values returned from functions.

```go
return nil, fmt.Errorf("could not connect: %w", err)
```

`%w` **wraps** the original error inside a new one with context. Creates a chain:
```
"migrations failed: could not create driver: dial tcp: connection refused"
```

`errors.Is()` can unwrap the chain to check for specific types:
```go
if errors.Is(err, migrate.ErrNoChange) { ... }
```

---

## 12. Go Pointers — When and Why

```go
cfg := Config{...}   // value — copies all fields when passed around
&cfg                 // pointer — 8 bytes regardless of struct size
```

**Why `Load()` returns `*Config`:**
1. **Efficiency** — one copy, many references
2. **Nilability** — `return nil, err` is idiomatic for failure; can't return a "zero Config"
3. **`*sql.DB` must be shared** — one pool across all goroutines, never copied

---

## 13. Blank Imports

```go
import _ "github.com/lib/pq"
```

Imports for **side effects only** — runs the package's `init()` function:
```go
// inside lib/pq — you never see this
func init() {
    sql.Register("postgres", &Driver{})
}
```
Now `sql.Open("postgres", dsn)` knows what "postgres" means. Same pattern for `source/file` in `migrate.go` — registers the `file://` migration source.

---

## 14. Password Hashing with bcrypt

**Never store plaintext passwords.** If your database leaks, every password is exposed.

**bcrypt** is a one-way hashing function designed specifically for passwords:

```go
// Store this in the database — not the original password
hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)

// On login — compare without ever storing the original
err := bcrypt.CompareHashAndPassword(hash, []byte("secret123"))
```

**Why bcrypt and not SHA-256?**
- bcrypt is intentionally **slow** (cost factor) — makes brute-force attacks take years
- bcrypt includes a **salt** automatically — same password hashes differently each time, preventing rainbow table attacks
- SHA-256 is fast and deterministic — terrible for passwords, great for data integrity

---

## 15. JWT — JSON Web Tokens

**The problem:** HTTP is stateless. After login, how does the server know who you are on the next request?

**Session cookies** (old way): Server stores session in a database, looks it up every request. Doesn't scale.

**JWT** (stateless): Server signs a token with a secret. Client stores it and sends it back with every request. Server validates the signature — no database lookup needed.

**Structure:** Three base64 parts separated by dots:
```
eyJhbGciOiJIUzI1NiJ9   .   eyJzdWIiOiJ1c2VyMSJ9   .   SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c
      HEADER                       PAYLOAD                        SIGNATURE
```

**Payload claims used in AtlasStore:**
```json
{
  "sub": "user-uuid",      ← subject (who this token is for)
  "exp": 1783373248,       ← expiry timestamp (Unix)
  "iat": 1783114048        ← issued-at timestamp
}
```

**Verification:** Server re-signs the header+payload with `JWT_SECRET` and compares to the signature. If they match, the token is authentic. If anyone tampers with the payload, the signature won't match.

**Security:** The `JWT_SECRET` must stay secret. Anyone with it can forge tokens.

---

## 16. HTTP Middleware Pattern

**The problem:** You need to run the same code (auth check, logging, rate limiting) before many different handlers without copy-pasting it into each one.

**Middleware** is a function that wraps another handler:

```go
func RequireAuth(secret string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {       // returns a new handler
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. Do your thing (validate token)
            if !valid {
                http.Error(w, "unauthorized", 401)
                return  // ← stops here, next handler never runs
            }
            // 2. Call the real handler
            next.ServeHTTP(w, r)
        })
    }
}
```

**How it wraps in router.go:**
```go
protected := auth.RequireAuth(cfg.JWTSecret)
mux.Handle("POST /objects", protected(http.HandlerFunc(objectHandler.Upload)))
```

Request hits `RequireAuth` → if token valid → hits `objectHandler.Upload`. The handler doesn't know or care about auth — separation of concerns.

---

## 17. Go Context — Passing Data Through a Request

**The problem:** The auth middleware validates a JWT and extracts `userID`. How does it pass that to the handler below it?

You can't use function parameters (middleware and handler have fixed HTTP signatures). You can't use global variables (concurrent requests would overwrite each other).

**Solution: `context.Context`** — a request-scoped key-value store that travels with the request:

```go
// In middleware — inject value
ctx := context.WithValue(r.Context(), UserIDKey, userID)
next.ServeHTTP(w, r.WithContext(ctx))

// In handler — read value
userID := r.Context().Value(auth.UserIDKey).(string)
```

**Why a private `contextKey` type?**
```go
type contextKey string        // private type
const UserIDKey contextKey = "userID"
```
If you used a plain `"userID"` string as the key, any package could overwrite it. The unexported type means only the `auth` package can read/write this specific key — no collisions.

---

## 18. Struct-Based Handlers (Dependency Injection)

**The problem:** Handler functions need access to the database and JWT secret. How do you get them there without global variables?

**Bad way — globals:**
```go
var DB *sql.DB  // global — untestable, invisible dependencies
```

**Good way — struct-based handlers:**
```go
type Handler struct {
    DB        *sql.DB
    JWTSecret string
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
    // h.DB and h.JWTSecret available here
}
```

`main.go` creates the handler with its dependencies explicitly:
```go
authHandler := &auth.Handler{
    DB:        database,
    JWTSecret: cfg.JWTSecret,
}
```

Dependencies are visible, testable, and no global state. This pattern is called **dependency injection**.

---

## 19. Two-Server Architecture

AtlasStore runs two separate HTTP servers. This is not accidental — it mirrors real distributed storage systems (S3, MinIO, GFS).

### Gateway (Control Plane) — port 8000
- Talks to clients and handles auth
- Has access to the database (knows about users, files, metadata)
- Orchestrates: decides which storage nodes get which chunks
- Exposed to the internet in production

### Storage Node (Data Plane) — port 9000
- Only talks to the gateway (never to clients directly)
- No database, no auth logic — just disk I/O
- Each node is a dumb file server: POST chunk → save, GET chunk → serve
- NOT exposed to the internet in production

**Why separate?**
1. **Scale independently** — need more disk space? Add storage nodes. High request volume? Scale the gateway. You can't do that if they're one binary.
2. **Different hardware** — gateway: CPU/RAM, storage nodes: large disks
3. **Fault isolation** — gateway crash ≠ data loss
4. **Replication** — same chunk gets sent to 3 different storage nodes. Only possible if they're separate addressable servers.

---

## 20. Why `io.Copy` Instead of `ReadAll`

```go
// BAD — loads entire chunk into RAM
data, _ := io.ReadAll(r.Body)
os.WriteFile(path, data, 0644)

// GOOD — streams from network to disk in 32KB increments
io.Copy(file, r.Body)
```

A 5MB chunk is small. But if 100 requests arrive simultaneously, `ReadAll` means 500MB in RAM at once. `io.Copy` uses a fixed ~32KB internal buffer regardless of file size. This is what makes storage systems efficient — you never need to hold the whole file in memory.

Same principle on `GetChunk`:
```go
io.Copy(w, file)  // streams file → response without loading it into RAM
```

---

## 21. Object Storage — What Problem It Solves

A regular filesystem (`/home/user/files/report.pdf`) doesn't scale:
- One disk fills up
- One machine goes down → files lost
- Hard to replicate across regions

**Object storage** treats every file as a flat **object** with a unique key, data bytes, and metadata. No folder hierarchy at the storage level. Any node can store any object — trivially distributable.

AtlasStore's model:
```
Object = objects table row + chunks table rows
Key    = objects.id (UUID)
Data   = chunk files spread across storage nodes
Meta   = name, size_bytes, content_type
```

---

## 22. Chunking — Why Files Are Split

**Without chunking:**
- 10GB file on one node → that node fills up
- Node goes down → file gone
- Can't parallelize download

**With chunking (5MB pieces):**
```
10GB file → 2048 chunks × 5MB
```

1. **Distribution** — chunks spread across nodes, no single node holds the whole file
2. **Replication** — each chunk stored on N nodes. One goes down → still have it
3. **Parallel I/O** — fetch chunk 0 from node A and chunk 1 from node B simultaneously
4. **Integrity** — SHA-256 hash each chunk. On download, re-hash and compare

**Reassembly:** `chunks.chunk_index` determines the order. `ORDER BY chunk_index` → concatenate bytes → original file.

---

## Concepts Still To Come

| Concept | When | Why It Matters |
|---|---|---|
| **SHA-256 hashing** | Week 3 | Fingerprinting chunks for integrity + deduplication |
| **Multipart file streaming** | Week 3 | Reading large uploads without loading into memory |
| **Goroutines & channels** | Week 3 | Uploading chunks to 3 nodes in parallel |
| **Consistent Hashing** | Phase 6 | Distributing chunks across nodes without a lookup table |
| **Raft Consensus** | Phase 7 | How distributed nodes agree on state |
| **gRPC** | Phase 5 | Binary protocol for internal node communication |
| **Prometheus metrics** | Phase 11 | Measuring system health and performance |
