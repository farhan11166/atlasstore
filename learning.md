# AtlasStore — Learning Reference

> **Purpose:** Concepts, patterns, and technologies explained from first principles.
> This file is your reference for understanding WHY things work the way they do.

---

## Table of Contents

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
14. [Object Storage — What Problem It Solves](#14-object-storage--what-problem-it-solves)
15. [Chunking — Why Files Are Split](#15-chunking--why-files-are-split)

---

## 1. Why Go?

Go was designed at Google for **network services and infrastructure**. That makes it a perfect fit for AtlasStore.

**What makes it good for this project:**

| Property | What It Means For You |
|---|---|
| **Compiled** | Produces a single binary — easy to ship to servers |
| **Goroutines** | Lightweight threads — handle thousands of simultaneous uploads cheaply |
| **Standard library** | Built-in HTTP server, crypto, file I/O — no framework needed |
| **Static typing** | Compiler catches bugs before runtime |
| **Fast compilation** | Feedback loop is seconds, not minutes |

Python would be simpler to write but would struggle at the concurrency scale that storage systems require. Rust would be faster but far harder to write. Go is the pragmatic middle ground — why MinIO, Docker, and Kubernetes are all written in Go.

---

## 2. Go Module System

When you see `go.mod`, it defines:

```
module github.com/farhan/atlasstore   ← the import path prefix for YOUR code
go 1.24.0                             ← minimum Go version
require (...)                         ← external dependencies with versions
```

**Why the import path looks like a GitHub URL:**
Go uses URLs as globally unique package names. It doesn't mean your code must be on GitHub — it's just a naming convention that guarantees no two packages in the world collide.

**`go.sum`:** A cryptographic lock file. Every dependency has a hash. If someone tampers with a library, `go.sum` fails. You never edit it manually.

---

## 3. Project Layout

### Why `cmd/`?

`cmd/` holds **executable entry points** — programs you can run.

```
cmd/gateway/main.go      → builds the API Gateway binary
cmd/storagenode/main.go  → builds the Storage Node binary
```

One repository, two separate runnable programs. This is the standard Go multi-binary pattern.

### Why `internal/`?

`internal/` is a **Go language enforcement rule**, not just a convention.

> Any package inside `internal/` can ONLY be imported by code in the parent directory.

```
github.com/farhan/atlasstore/internal/config  ← can ONLY be imported by
github.com/farhan/atlasstore/...              ← code under atlasstore/
```

If someone tries to import your internal packages from another module, the Go compiler refuses. This enforces that your implementation details stay private.

### Why `pkg/`?

`pkg/` is for code you'd eventually want to share — things that would make sense as a standalone library. It's the opposite of `internal/`.

---

## 4. Configuration Management

### The Problem

You need values like database passwords and JWT secrets available at runtime. You can't hardcode them — that's a security disaster (imagine pushing to GitHub). You need a way to inject them without touching code.

### The Solution: Environment Variables

The OS provides a key-value store called **environment variables**. Any process can read them. This is the 12-factor app principle: **store config in the environment, not in code**.

### The `.env` File + `godotenv`

In development, setting environment variables manually every time is tedious. The `.env` file is a convenience — `godotenv.Load()` reads it and injects the values into the process environment at startup.

```
godotenv.Load()
    ↓
Reads KEY=VALUE pairs from .env
    ↓
Calls os.Setenv(KEY, VALUE) for each one
    ↓
Now os.Getenv("KEY") works anywhere in your program
```

**Why `.env` is in `.gitignore`:** It contains real secrets. You never commit secrets. In production, environment variables are set directly on the server or via a secrets manager.

### The Typed `Config` Struct

Raw env vars are all strings. Stringly-typed code is fragile:

```go
// Bad — you have to remember what format this is in, everywhere
port := os.Getenv("GATEWAY_PORT")

// Good — the type tells you everything
cfg.GatewayPort  // string: "8080"
cfg.ChunkSizeMB  // int: 5
```

`config.Load()` does the conversion once at startup. If it fails (bad value, missing secret), the program refuses to start. **Fail fast at startup, not silently mid-operation.**

---

## 5. Database — PostgreSQL Fundamentals

### What is a Relational Database?

Data is stored in **tables** (like spreadsheets). Tables are connected to each other through **relationships** (foreign keys). SQL (Structured Query Language) is how you talk to it.

### Why PostgreSQL?

PostgreSQL is the gold standard for production Go backend databases. It has:
- **UUIDs natively** — via the `pgcrypto` extension
- **JSONB** — store semi-structured data if needed
- **Full ACID compliance** — transactions are safe
- **Excellent Go drivers** — `lib/pq`, `pgx`

### `database/sql` vs `pgx`

AtlasStore uses `database/sql` (standard library) with `lib/pq` as the driver.

```
database/sql    ← Go standard library interface (generic)
lib/pq          ← PostgreSQL-specific implementation of that interface
```

`database/sql` defines HOW you talk to databases. `lib/pq` defines HOW it talks to Postgres specifically. You write code against `database/sql` — if you ever switch to `pgx`, only the driver changes.

---

## 6. Database Migrations

### The Problem

Your database schema evolves as you build features. Week 1 you create `users`. Week 2 you need to add a `role` column. Week 3 you realize you need a new `buckets` table.

**How do you track what's been applied to which database?** You can't just re-run `CREATE TABLE` — it'll fail if the table exists. You can't always remember what version your staging database is at vs. production.

### The Solution: Versioned Migration Files

Each change to the database is a numbered, timestamped SQL file:

```
000001_init_schema.up.sql      ← creates the initial tables
000002_add_role_to_users.up.sql ← adds a column to users
000003_add_buckets.up.sql       ← adds a new table
```

`golang-migrate` keeps track of which files have been applied in a special `schema_migrations` table inside your database. On next startup:

```
Applied: 000001 ✓
Applied: 000002 ✓
Pending: 000003   ← applies this automatically
```

### The `.up` / `.down` Pair

Every migration needs a reverse:

```
000001_init_schema.up.sql    ← apply (CREATE TABLE)
000001_init_schema.down.sql  ← rollback (DROP TABLE)
```

If you apply migration 3 and it causes a bug, you run `migrate down` to reverse it. The tables must be dropped in reverse dependency order:

```sql
DROP TABLE IF EXISTS chunks;   ← depends on objects, drop first
DROP TABLE IF EXISTS objects;  ← depends on users, drop second
DROP TABLE IF EXISTS users;    ← drop last
```

---

## 7. Connection Pooling

### The Problem

Opening a new database connection takes time — TCP handshake, authentication, SSL negotiation. If your API creates a new connection for every HTTP request, it's slow and you'll quickly exhaust PostgreSQL's connection limit.

### The Solution: A Connection Pool

`database/sql` maintains a **pool** of pre-opened connections and reuses them.

```
Request 1 → borrows connection from pool → does query → returns connection
Request 2 → borrows same connection       → does query → returns connection
Request 3 → borrows connection from pool  (parallel with request 2)
```

**`SetMaxOpenConns(25)`** — at most 25 connections open at once. Requests beyond that wait in a queue.

**`SetMaxIdleConns(10)`** — keep 10 connections alive even when idle, so they're ready immediately for the next request.

**The tradeoff:** More connections = more memory on the PostgreSQL server. 25 is a reasonable starting point.

---

## 8. UUIDs as Primary Keys

### What's wrong with auto-increment integers (1, 2, 3...)?

1. **Predictability** — If your user ID is `42`, an attacker can try `41`, `40`, `43`. UUIDs are random and unguessable.
2. **Distributed systems problem** — If you ever have multiple database nodes generating IDs simultaneously, integers will collide. Two nodes might both generate `42`. UUIDs won't collide because they're 128-bit random values.
3. **No sequential scraping** — Can't crawl your API by incrementing IDs.

### `gen_random_uuid()` from `pgcrypto`

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
```

PostgreSQL generates a cryptographically random UUID for every new row automatically. You never need to supply an ID in your INSERT statements.

A UUID looks like: `550e8400-e29b-41d4-a716-446655440000`

---

## 9. Foreign Keys and Cascading Deletes

### Foreign Keys

A foreign key is a guarantee: **"this value must exist in another table"**.

```sql
user_id UUID NOT NULL REFERENCES users(id)
```

This means: `objects.user_id` MUST be a valid `id` from the `users` table. You cannot insert an object for a user that doesn't exist. The database enforces this for you — your application code doesn't have to.

### `ON DELETE CASCADE`

```sql
user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
```

> "When a user is deleted, automatically delete all their objects too."

Without `CASCADE`, deleting a user would FAIL if they have any objects (the database protects referential integrity). With CASCADE, the delete propagates:

```
DELETE user 42
  → automatically DELETE all objects where user_id = 42
    → automatically DELETE all chunks where object_id IN (deleted objects)
```

One SQL statement, entire tree cleaned up safely.

---

## 10. What is a DSN?

DSN = **Data Source Name**. It's a connection string that tells the driver everything it needs to connect.

```
host=localhost port=5432 user=atlasstore password=atlaspassword dbname=atlasstore sslmode=disable
```

| Part | Meaning |
|---|---|
| `host` | Where PostgreSQL is running |
| `port` | TCP port (PostgreSQL default: 5432) |
| `user` | Database username |
| `password` | Database password |
| `dbname` | Which database to connect to |
| `sslmode=disable` | Don't require SSL (fine for local dev, require in production) |

In AtlasStore, the DSN is assembled in `config.go` from individual env vars and stored as `cfg.DBDSN`. This keeps the env vars human-readable while giving the driver the format it needs.

---

## 11. Error Wrapping in Go

Go does not have exceptions. Errors are values returned from functions.

```go
// A function that can fail returns (result, error)
db, err := sql.Open("postgres", dsn)
if err != nil {
    // handle it
}
```

### `fmt.Errorf` with `%w`

```go
return nil, fmt.Errorf("could not connect: %w", err)
```

`%w` **wraps** the original error inside a new one with more context. This creates an error chain:

```
"migrations failed: could not create driver: dial tcp: connection refused"
  └── your message  └── migrate's message  └── OS-level message
```

This is invaluable for debugging. Instead of just `"connection refused"`, you know WHERE in your code it failed.

**`errors.Is()`** can unwrap the chain to check for specific error types:
```go
if errors.Is(err, migrate.ErrNoChange) { ... }
```

---

## 12. Go Pointers — When and Why

### What is a pointer?

A pointer stores the **memory address** of a value, not the value itself.

```go
cfg := Config{...}   // Config value (copy)
&cfg                 // *Config pointer (address of cfg)
```

### Why `Load()` returns `*Config` not `Config`

```go
func Load() (*Config, error)
```

1. **Efficiency** — `Config` has ~10 fields. Returning a value copies all 10 fields every time. A pointer is always 8 bytes regardless of struct size.

2. **Nilability** — A pointer can be `nil`, a value cannot. Returning `nil, err` when something goes wrong is idiomatic Go. You can't return a "zero Config" to mean "failed".

3. **Shared mutation** — If multiple parts of your code hold `*Config`, they all see the same data. (For config this doesn't matter much, but for database connections `*sql.DB` it's essential — you want one pool shared everywhere.)

---

## 13. Blank Imports

```go
import _ "github.com/lib/pq"
```

The `_` means: **"import this package for its side effects only, don't use it directly"**.

When `lib/pq` is imported, its `init()` function runs automatically:

```go
// inside lib/pq (you never see this)
func init() {
    sql.Register("postgres", &Driver{})
}
```

This registers the postgres driver with `database/sql`'s global registry. Now when you call `sql.Open("postgres", dsn)`, it knows what "postgres" means.

Same pattern in `migrate.go`:
```go
_ "github.com/golang-migrate/migrate/v4/source/file"
```

Registers the `file://` migration source so `golang-migrate` can read `.sql` files from disk.

---

## 14. Object Storage — What Problem It Solves

### Traditional file storage

A regular filesystem (ext4, NTFS) stores files in a hierarchy: `/home/user/documents/report.pdf`. It's great for a single machine but doesn't scale.

**Problems at scale:**
- One disk fills up
- One machine goes down → files lost
- Concurrent writes to the same directory cause contention
- Hard to replicate across geographic regions

### Object Storage

An object store treats every file as an **object** with:
- A unique **key** (like a UUID or path)
- Arbitrary **data** (the bytes)
- **Metadata** (size, content type, creation time)

Objects are stored in **flat namespace** — no folder hierarchy at the storage level (the "folders" in S3 are just key prefixes). This makes distribution trivial: any node can store any object.

AtlasStore's data model maps directly:
```
Object = objects table row + chunks table rows
Key    = objects.id (UUID)
Data   = chunk files spread across storage nodes
Meta   = objects.name, objects.size_bytes, objects.content_type
```

---

## 15. Chunking — Why Files Are Split

### The Problem With Storing Files Whole

Imagine a user uploads a 10 GB video file to a single storage node:

1. **That node fills up** — nowhere to put more data
2. **That node goes down** — the file is gone
3. **Download is sequential** — can't parallelize reads

### The Solution: Chunk the File

Split every file into fixed-size pieces (e.g., 5 MB each):

```
10 GB file → 2048 chunks of 5 MB each
```

Now:
1. **Chunks spread across nodes** — no single node holds the whole file
2. **Replication** — each chunk is stored on N nodes (replication factor). One node dies → still have it on another
3. **Parallel download** — fetch chunk 0 from node A and chunk 1 from node B simultaneously → faster
4. **Integrity** — SHA-256 hash each chunk. On download, re-hash and compare. If they differ, the data is corrupted

**In AtlasStore's schema:**

```sql
chunks.chunk_index    ← determines reassembly order (0, 1, 2...)
chunks.hash           ← SHA-256 fingerprint for integrity verification
chunks.node_address   ← which storage node has this chunk
chunks.size           ← how many bytes (last chunk may be smaller)
```

To download a file:
1. Query `chunks WHERE object_id = ?` ORDER BY `chunk_index`
2. For each chunk: fetch bytes from `node_address`
3. Verify SHA-256 hash
4. Concatenate in order → original file

---

## Concepts Still To Come

| Concept | When You'll Encounter It | Why It Matters |
|---|---|---|
| **bcrypt** | Week 2 (Auth) | How to store passwords safely — never store plaintext |
| **JWT (JSON Web Tokens)** | Week 2 (Auth) | Stateless authentication — server doesn't store sessions |
| **HTTP Middleware** | Week 2 (Auth) | How to intercept requests before they reach handlers |
| **Goroutines & channels** | Week 3+ (parallel chunks) | How Go handles concurrency |
| **SHA-256 hashing** | Week 3 (storage) | Cryptographic fingerprinting for data integrity |
| **Consistent Hashing** | Phase 6 | How to distribute chunks across nodes without a central map |
| **Raft Consensus** | Phase 7 | How distributed systems agree on state without a single master |
| **gRPC** | Phase 5 | Binary protocol for inter-service communication, faster than REST |
