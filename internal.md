# AtlasStore — Internal Reference

> **Purpose:** How THIS project is structured — file connections, request flows, and implementation details.
> This file is your reference for understanding HOW the project is built.

---

## Directory Map

```
AtlasStore/
├── cmd/
│   ├── gateway/
│   │   └── main.go          ← Entry point: API Gateway process (port 8000)
│   └── storagenode/
│       └── main.go          ← Entry point: Storage Node process (port 9000)
│
├── internal/                ← Private application code
│   ├── config/
│   │   └── config.go        ← Reads .env → typed *Config struct
│   ├── db/
│   │   ├── db.go            ← Opens PostgreSQL connection pool
│   │   ├── migrate.go       ← Runs SQL migrations on startup
│   │   └── user_repo.go     ← CreateUser, GetUserByEmail DB queries
│   ├── auth/
│   │   ├── handler.go       ← Register + Login HTTP handlers
│   │   └── middleware.go    ← JWT validation middleware (used in Week 3+)
│   ├── api/
│   │   └── router.go        ← HTTP route wiring (all routes defined here)
│   └── storage/
│       └── disk.go          ← Chunk save/read/delete on local disk
│
├── migrations/
│   ├── 000001_init_schema.up.sql    ← Creates users, objects, chunks tables
│   └── 000001_init_schema.down.sql ← Drops tables (rollback)
│
├── docker-compose.yml       ← Runs PostgreSQL on host port 5433
├── go.mod                   ← Module: github.com/farhan/atlasstore
├── .env                     ← Runtime config (ports, DB creds, JWT secret)
└── data/                    ← Created at runtime by storage node
    └── node1/               ← Chunk files stored here (named by SHA-256 hash)
```

---

## Two-Server Architecture

AtlasStore runs as **two separate processes** — this is intentional distributed systems design:

```
CLIENT
  │  HTTP :8000
  ▼
┌─────────────────────────────────────┐
│  GATEWAY (cmd/gateway/main.go)      │  ← "Brain" / Control Plane
│  - Handles auth (JWT)               │
│  - Talks to PostgreSQL              │
│  - Orchestrates chunk distribution  │
└─────────────────────────────────────┘
  │  Internal HTTP calls to :9000
  ▼
┌─────────────────────────────────────┐
│  STORAGE NODE (cmd/storagenode/)    │  ← "Muscle" / Data Plane
│  - No auth, no database             │
│  - Just saves/reads bytes on disk   │
│  - One node per machine in prod     │
└─────────────────────────────────────┘
```

In production: 1 gateway + 3+ storage nodes on separate machines. Locally: 2 terminals.

---

## File-by-File Breakdown

### `.env`

| Variable | Used By | Purpose |
|---|---|---|
| `GATEWAY_PORT` | config → main.go | Port the API Gateway listens on |
| `STORAGE_NODE_PORT` | config → storagenode | Port each storage node listens on |
| `DB_*` | config → db.go | Assembled into PostgreSQL DSN |
| `JWT_SECRET` | config → auth/handler.go | Signs and verifies JWT tokens |
| `JWT_EXPIRY_HOURS` | config → auth/handler.go | Token validity duration |
| `CHUNK_SIZE_MB` | config → (Week 3) | Max size per chunk |
| `REPLICATION_FACTOR` | config → (Week 3) | How many nodes receive each chunk |
| `STORAGE_DATA_DIR` | storagenode/main.go | Overrides default `./data/chunks` dir |

---

### `internal/config/config.go`

**Flow:**
```
godotenv.Load()          → reads .env into os environment
os.LookupEnv(key)        → getEnv() reads each var with fallback default
Config{} struct literal  → all fields populated
Validation               → fatal if JWT_SECRET or DB_PASSWORD missing
strconv.Atoi()           → parses int fields (expiry, chunk size, replication)
fmt.Sprintf(DSN)         → "host=... port=... user=... password=... dbname=..."
return &cfg, nil
```

---

### `internal/db/db.go`

**Flow:**
```
sql.Open("postgres", dsn)  → registers driver+DSN (does NOT connect yet)
db.Ping()                  → actually tests the connection
db.SetMaxOpenConns(25)     → connection pool limit
db.SetMaxIdleConns(10)     → keep 10 warm connections ready
return db, nil
```

---

### `internal/db/migrate.go`

**Flow:**
```
postgres.WithInstance(db)          → wraps *sql.DB for migrate library
migrate.NewWithDatabaseInstance()  → reads .sql files from migrations/
m.Up()                             → applies all unapplied .up.sql files
if err == ErrNoChange → ok         → already up to date, not an error
```

---

### `internal/db/user_repo.go`

Two functions:

| Function | SQL | Returns |
|---|---|---|
| `CreateUser(db, email, hash)` | `INSERT INTO users ... RETURNING id` | `string` (UUID) |
| `GetUserByEmail(email, db)` | `SELECT ... FROM users WHERE email=$1` | `*User` or `nil` |

`nil, nil` from `GetUserByEmail` means "user not found" — not a system error.

---

### `internal/auth/handler.go`

**Register flow (`POST /auth/register`):**
```
Decode JSON body (email, password)
Check email not already used → GetUserByEmail
bcrypt.GenerateFromPassword(password) → hash
CreateUser(email, hash) → get UUID
generateToken(UUID) → signed JWT
Return 201 + {"token": "..."}
```

**Login flow (`POST /auth/login`):**
```
Decode JSON body
GetUserByEmail → get user row
bcrypt.CompareHashAndPassword(stored_hash, input_password)
generateToken(user.ID) → signed JWT
Return 200 + {"token": "..."}
```

**Security rule:** Both wrong-email and wrong-password return the same `"invalid credentials"` — never reveal which one failed.

---

### `internal/auth/middleware.go`

**Status: Built, NOT yet wired to any routes.**

It will be activated in Week 3 when object routes are added in `router.go`.

**Flow when active:**
```
Read "Authorization" header
Split "Bearer <token>" → extract token string
jwt.Parse(token, verify HMAC signature)
token.Claims["sub"] → extract userID
context.WithValue(r.Context(), UserIDKey, userID)
next.ServeHTTP(w, r.WithContext(ctx))  ← passes modified request downstream
```

**How handlers use the injected userID (Week 3):**
```go
userID := r.Context().Value(auth.UserIDKey).(string)
```

---

### `internal/api/router.go`

All routes defined here. `main.go` calls `NewRouter()` once and passes the result to `http.ListenAndServe`.

**Current routes:**
```
POST /auth/register  →  authHandler.Register   (public)
POST /auth/login     →  authHandler.Login      (public)
```

**Upcoming Week 3 routes (commented out):**
```
POST   /objects       →  protected → objectHandler.Upload
GET    /objects       →  protected → objectHandler.List
GET    /objects/{id}  →  protected → objectHandler.Download
DELETE /objects/{id}  →  protected → objectHandler.Delete
```

`protected` = `auth.RequireAuth(cfg.JWTSecret)` wrapping the handler.

---

### `internal/storage/disk.go`

The storage node's handler. Chunks are stored as plain files named by their SHA-256 hash.

| Method | Route | What it does |
|---|---|---|
| `SaveChunk` | `POST /chunk` | Reads `X-Chunk-Hash` header, streams body → file on disk |
| `GetChunk` | `GET /chunk/{hash}` | Opens file by hash name, streams to response |
| `DeleteChunk` | `DELETE /chunk/{hash}` | `os.Remove()` the file |
| `Health` | `GET /health` | Returns `{"status":"ok"}` — gateway polls this (Week 3) |

**Key:** `io.Copy(file, r.Body)` streams bytes directly from network to disk — never loads the whole chunk into RAM.

---

### `cmd/gateway/main.go`

**Startup sequence:**
```
1. config.Load()                   ← read .env
2. db.Connect(cfg.DBDSN)           ← open postgres pool
3. db.RunMigrations(database)      ← apply pending .up.sql files
4. api.NewRouter(cfg, database)    ← wire all routes
5. http.ListenAndServe(:8000)      ← start accepting requests
```

### `cmd/storagenode/main.go`

**Startup sequence:**
```
1. config.Load()                   ← read .env
2. os.Getenv("STORAGE_DATA_DIR")   ← or default to ./data/chunks
3. storage.NodeHandler{DataDir}    ← create handler with data dir
4. http.NewServeMux() + routes     ← wire chunk endpoints
5. http.ListenAndServe(:9000)      ← start accepting chunk requests
```

---

## Request Flow — Auth (Working Now)

```
curl POST /auth/register
  │
  ▼
router.go → authHandler.Register()
  │
  ├── db.GetUserByEmail()     ← check not duplicate
  ├── bcrypt.Generate()       ← hash the password
  ├── db.CreateUser()         ← INSERT into users table
  └── jwt.NewWithClaims()     ← sign token with JWTSecret
  │
  ▼
{"token": "eyJ..."}
```

---

## Request Flow — Object Upload (Week 3 — Not Yet Built)

```
curl POST /objects (with Bearer token + file body)
  │
  ▼
router.go → RequireAuth middleware
  │  validates JWT, injects userID into context
  ▼
objectHandler.Upload()
  │
  ├── read file from request body
  ├── split into 5MB chunks
  ├── SHA-256 hash each chunk
  ├── POST /chunk to storage node (with X-Chunk-Hash header)
  ├── INSERT into objects table
  └── INSERT into chunks table (chunk_index, hash, node_address)
  │
  ▼
{"id": "uuid-of-object"}
```

---

## What Is NOT Built Yet

| Component | Location | Needed For |
|---|---|---|
| Object handlers | `internal/api/` | Upload, download, list, delete |
| Middleware wired | `router.go` (uncomment) | Protecting object routes |
| DB object/chunk repo | `internal/db/` | Saving file + chunk metadata |
| SHA-256 chunking logic | `internal/api/` | Breaking files into pieces |
| Health-check polling | gateway → storage nodes | Detecting dead nodes |
| Web dashboard | `web/` or `static/` | Week 4 frontend |
