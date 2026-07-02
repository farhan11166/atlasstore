# AtlasStore — Internal Reference

> **Purpose:** How THIS project is structured — file connections, request flows, and implementation details.
> This file is your reference for understanding HOW the project is built.

---

## Directory Map

```
AtlasStore/
├── cmd/
│   ├── gateway/
│   │   └── main.go          ← Entry point for the API Gateway process
│   └── storagenode/
│       └── (empty)          ← Future: Entry point for Storage Node process
│
├── internal/                ← Private application code (not importable by outside packages)
│   ├── config/
│   │   └── config.go        ← Reads .env, returns a typed *Config struct
│   ├── db/
│   │   ├── db.go            ← Opens PostgreSQL connection, returns *sql.DB
│   │   └── migrate.go       ← Runs pending SQL migrations on startup
│   ├── auth/                ← (empty) Future: JWT handlers, middleware
│   ├── api/                 ← (empty) Future: HTTP route handlers
│   └── storage/             ← (empty) Future: Local disk chunk read/write
│
├── migrations/
│   ├── 000001_init_schema.up.sql    ← Creates: users, objects, chunks tables
│   └── 000001_init_schema.down.sql ← Drops:   chunks, objects, users (reverse order)
│
├── api/                     ← (empty) Future: OpenAPI / Protobuf definitions
├── pkg/                     ← (empty) Future: Reusable library code
│
├── docker-compose.yml       ← Spins up PostgreSQL container
├── go.mod                   ← Module: github.com/farhan/atlasstore
├── go.sum                   ← Dependency lock file
└── .env                     ← Runtime secrets and config values (never commit this)
```

---

## File-by-File Breakdown

### `.env`
The single source of truth for all runtime configuration.

| Variable | Used By | Purpose |
|---|---|---|
| `GATEWAY_PORT` | config.go → main.go | Port the API Gateway HTTP server listens on |
| `STORAGE_NODE_PORT` | config.go | Port each storage node listens on |
| `DB_HOST/PORT/USER/PASSWORD/NAME` | config.go → db.go | Assembled into the DSN connection string |
| `JWT_SECRET` | config.go → future auth | Signs and verifies JWT tokens |
| `JWT_EXPIRY_HOURS` | config.go | How long a login token is valid |
| `CHUNK_SIZE_MB` | config.go → future storage | Max size of each file chunk |
| `REPLICATION_FACTOR` | config.go → future gateway | How many nodes receive each chunk |

---

### `internal/config/config.go`

**What it does:** Loads `.env` into a Go struct so all other code gets typed values instead of raw strings.

**Key function:** `Load() (*Config, error)`

**Flow:**
```
godotenv.Load()          ← reads .env file into os environment
    ↓
os.LookupEnv(key)        ← getEnv() reads each variable with a fallback default
    ↓
Config{} struct literal  ← all string fields populated
    ↓
Validation               ← returns error if JWT_SECRET or DB_PASSWORD is empty
    ↓
strconv.Atoi()           ← parses integer fields (JWTExpiryHours, ChunkSizeMB, ReplicationFactor)
    ↓
fmt.Sprintf(DSN)         ← assembles: "host=... port=... user=... password=... dbname=... sslmode=disable"
    ↓
return &cfg, nil
```

**Why `*Config` (pointer)?** Avoids copying the whole struct every time it's passed to another function. One struct, many references.

---

### `internal/db/db.go`

**What it does:** Opens a connection pool to PostgreSQL and verifies it's reachable.

**Key function:** `Connect(dsn string) (*sql.DB, error)`

**Flow:**
```
sql.Open("postgres", dsn)   ← registers the driver and DSN, does NOT connect yet
    ↓
db.Ping()                   ← THIS is what actually connects and tests the DB
    ↓
db.SetMaxOpenConns(25)      ← limits simultaneous DB connections (prevents overload)
db.SetMaxIdleConns(10)      ← keeps 10 connections warm for reuse
    ↓
return db, nil              ← *sql.DB is safe for concurrent use across goroutines
```

**Important:** `sql.Open()` does NOT connect. It just validates the driver name and DSN format. `Ping()` is mandatory to confirm the DB is actually up.

---

### `internal/db/migrate.go`

**What it does:** Applies any pending `.up.sql` migration files to the database on startup.

**Key function:** `RunMigrations(db *sql.DB, migrationsPath string) error`

**Flow:**
```
postgres.WithInstance(db, ...)      ← wraps *sql.DB into a migrate-compatible driver
    ↓
migrate.NewWithDatabaseInstance()   ← reads migration files from migrationsPath
    ↓
m.Up()                              ← applies all unapplied migrations in order
    ↓
if err == migrate.ErrNoChange       ← already up to date, not a real error — ignore it
```

**Migration file naming:**
```
000001_init_schema.up.sql    ← number prefix = execution order
000001_init_schema.down.sql  ← must exist alongside .up for rollback support
000002_...up.sql             ← next migration, runs after 000001
```

---

### `migrations/000001_init_schema.up.sql`

**Creates three tables:**

```
users
  id          UUID (PK, auto-generated)
  email       TEXT UNIQUE NOT NULL
  password    TEXT NOT NULL          ← stores bcrypt hash, never plaintext
  created_at  TIMESTAMP

objects                              ← one row per uploaded file
  id          UUID (PK)
  user_id     UUID → users.id        ← FK: who owns this file
  name        TEXT                   ← original filename
  size_bytes  BIGINT
  content_type TEXT
  created_at  TIMESTAMPTZ

chunks                               ← one row per piece of a file
  id          UUID (PK)
  object_id   UUID → objects.id      ← FK: which file this chunk belongs to
  chunk_index INT                    ← 0, 1, 2... ordering for reassembly
  hash        TEXT                   ← SHA-256 of chunk content (integrity check)
  size        BIGINT
  node_address TEXT                  ← "http://storagenode1:9000" where chunk lives
  created_at  TIMESTAMPTZ
  UNIQUE(object_id, chunk_index)     ← can't have duplicate chunk #3 for same file
```

**Relationship diagram:**
```
users (1) ──────< objects (many)
                      │
                      └──────< chunks (many)
```

---

### `cmd/gateway/main.go`

**What it does:** The startup sequence for the entire API Gateway process.

**Current flow:**
```
config.Load()               ← step 1: read all config from .env
    ↓
db.Connect(cfg.DBDSN)       ← step 2: open postgres connection pool
    ↓
db.RunMigrations(database)  ← step 3: bring DB schema up to date
    ↓
[TODO: start HTTP server]   ← step 4: not yet implemented
```

**Design principle:** `main()` is the composition root. It creates all dependencies and passes them down. Nothing else calls `config.Load()` — only `main()` does.

---

## 🐛 Bugs Found In Your Code

These will cause compile errors or silent runtime failures. Fix these before running.

### `internal/config/config.go`

| Line | Bug | Fix |
|---|---|---|
| 18 | `DBPassowrd string` — typo in field name | Change to `DBPassword string` |
| 42 | `DBPassword:` in struct literal — this will fail to compile because the field is named `DBPassowrd` | Fix the field name on line 18 |
| 55 | `getEnv("JWT_EXPIRY")` — only one argument, but `getEnv` requires two | `getEnv("JWT_EXPIRY_HOURS", "72")` |
| 63-66 | `ChunkSizeMB` is parsed **twice** (duplicate block) | Delete lines 63-66, keep only 59-62 |
| 77 | `return cfg, nil` returns a value, but signature says `*Config` | Change to `return &cfg, nil` |

### `cmd/gateway/main.go`

| Line | Bug | Fix |
|---|---|---|
| 16 | `db.connect(...)` — lowercase `c`, Go won't find it | `db.Connect(...)` (capital C) |
| 16 | `cfg.DBSDN` — typo | `cfg.DBDSN` |
| 24 | `" file://./migrations"` — leading space in string | `"file://./migrations"` (remove the space) |

### `migrations/000001_init_schema.up.sql`

| Line | Bug | Fix |
|---|---|---|
| 6 | `TIMESTAMP NOT NULL` on `users.created_at` | Should be `TIMESTAMPTZ` (with timezone) to match `objects` and `chunks` for consistency |

---

## Request Flow (Future — When HTTP server is added)

```
Client
  │
  ▼
cmd/gateway/main.go        ← starts HTTP server on GATEWAY_PORT
  │
  ▼
internal/api/              ← route handler receives request
  │  reads JWT from header
  ▼
internal/auth/middleware   ← validates JWT, extracts user_id
  │
  ▼
internal/api/handler       ← business logic
  │  breaks file into chunks
  │  picks storage nodes
  ▼
internal/db/               ← saves object + chunk metadata to PostgreSQL
  │
  ▼
Storage Node (HTTP)        ← receives chunk bytes via POST /chunk
  │
  ▼
internal/storage/          ← writes chunk to local disk
```

---

## What Is NOT Built Yet

| Component | Location | Needed For |
|---|---|---|
| HTTP server | `cmd/gateway/main.go` | Any API to work |
| Route handlers | `internal/api/` | Upload, download, delete, list |
| Auth handlers | `internal/auth/` | Register, login endpoints |
| JWT middleware | `internal/auth/` | Protecting routes |
| Chunk storage logic | `internal/storage/` | Writing/reading chunk files to disk |
| Storage node server | `cmd/storagenode/` | Receiving chunks from gateway |
| DB repository layer | `internal/db/` | Typed CRUD functions for users/objects/chunks |
