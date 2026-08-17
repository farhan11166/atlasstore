# AtlasStore

> 🎯 **Status: Phase 6 Complete — Consistent Hashing & Rebalancing** | Actively building Phase 7 (Raft Consensus)

> A distributed object storage platform built in Go — inspired by Amazon S3 and MinIO.

AtlasStore separates the **Control Plane** (API Gateway) from the **Data Plane** (Storage Nodes) to orchestrate chunked, encrypted, and replicated file storage across a cluster. It was built from scratch as a learning project in distributed systems.

---

## ✅ What's Built So Far

| Phase | Feature | Status |
|---|---|---|
| 1 | Two-server architecture (Gateway + Storage Node) | ✅ Done |
| 1 | JWT Auth (Register / Login) | ✅ Done |
| 1 | PostgreSQL metadata layer | ✅ Done |
| 1 | Web Dashboard (upload, download, delete) | ✅ Done |
| 2 | Chunking engine (5MB chunks, parallel upload/download) | ✅ Done |
| 2 | SHA-256 integrity verification on download | ✅ Done |
| 2 | Multipart upload support | ✅ Done |
| 3 | Storage node auto-registration with Gateway | ✅ Done |
| 3 | Background health checker (gRPC ping) | ✅ Done |
| 4 | Replication (N copies per chunk) | ✅ Done |
| 4 | Fallback download from replica on failure | ✅ Done |
| 5 | gRPC migration (internal HTTP → gRPC) | ✅ Done |
| 6 | Consistent Hashing ring with virtual nodes | ✅ Done |
| 6 | Deterministic chunk placement via Hash Ring | ✅ Done |
| 6 | Background Rebalancing Worker | ✅ Done |
| 8* | Encryption at rest (AES-GCM) | ✅ Done |

---

## 🏗️ Architecture

```text
       [Web Dashboard]
              │ (HTTP / JSON)
              ▼
   ┌──────────────────────────────────────┐
   │          API Gateway                 │ ← "Brain" / Control Plane
   │  - JWT Auth                          │
   │  - Consistent Hash Ring (50 vNodes)  │
   │  - Background Health Checker         │
   │  - Background Rebalancer             │ ── (Metadata) ──► [ PostgreSQL DB ]
   │  - Encryption (AES-GCM)              │
   └──────────────────────────────────────┘
              │ (internal gRPC)
      ┌───────┼───────┐
      ▼       ▼       ▼
   [Node A] [Node B] [Node C]    ← "Muscles" / Data Plane
     Disk    Disk     Disk       ← Encrypted chunk files (named by SHA-256 hash)
```

### How Chunk Placement Works

1. File is split into **5MB chunks** (configurable).
2. Each chunk is **AES-GCM encrypted** before leaving the Gateway.
3. SHA-256 hash of the **plaintext** chunk is computed → used as the chunk's filename and lookup key.
4. The **Consistent Hash Ring** maps the chunk hash → N storage node addresses (where N = replication factor).
5. Chunks are saved to those nodes in parallel via **gRPC**.
6. If a node is added or removed, the **Rebalancer** (runs every 60s) detects misplaced chunks and migrates them without downtime.

---

## 🚀 Performance & Load Testing

AtlasStore has been rigorously load-tested to find its breaking point on local hardware. The system architecture (Go Goroutines + gRPC + PostgreSQL) is designed for massive horizontal scalability, but a single local instance provides incredible baseline performance.

**Test Setup:**
- **Concurrency:** 2,000 simultaneous virtual users
- **Duration:** 15 seconds
- **Traffic:** Continuous stream of Multi-part File Uploads
- **Hardware:** 12th Gen Intel(R) Core(TM) i5-1235U (12 Cores), 8GB RAM

**Results (Single Machine Peak):**
| Concurrency (Virtual Users) | Total Requests (15s) | Success Rate | Throughput (Req/Sec) | Notes |
|---|---|---|---|---|
| **100** | ~3,260 | 100.00% | ~210 req/s | Perfect stability. |
| **500** | ~3,464 | 99.83% | ~208 req/s | Slight resource contention. |
| **2,000** | ~4,883 | 92.91% | ~215 req/s | OS file descriptors and DB connections exhausted. |

*Note: Each recorded "Request" is a full multi-part file upload consisting of 3 separate HTTP round-trips. The Gateway actually processed over 600 HTTP requests per second at peak.*

**Bottleneck Analysis:**
At 2,000 concurrent users, the Go application itself did not crash. The 7% error rate was caused by hitting physical hardware limits (OS File Descriptors and PostgreSQL `max_connections` pool exhaustion). To scale beyond this, the architecture supports deploying the Gateway behind a load balancer and horizontally scaling the PostgreSQL database.

---

## 🛠️ Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.24 (standard library + minimal dependencies) |
| Database | PostgreSQL (via `lib/pq` + `golang-migrate`) |
| Internal Comms | gRPC + Protocol Buffers (`pkg/pb`) |
| Auth | `bcrypt` (passwords) + `golang-jwt/jwt/v5` (tokens) |
| Encryption | AES-GCM 256-bit (at rest) |
| Hashing | SHA-256 (chunk integrity + consistent ring placement) |
| Frontend | Vanilla HTML/CSS/JavaScript |

---

## ⚙️ Getting Started

### Prerequisites

- [Go 1.24+](https://go.dev/)
- [Docker & Docker Compose](https://www.docker.com/)

### 1. Setup Database

```bash
docker compose up -d
```

### 2. Configure Environment

Ensure your `.env` file exists:

```env
DB_HOST=localhost
DB_PORT=5433
DB_USER=atlasstore
DB_PASSWORD=atlaspassword
DB_NAME=atlasstore
DB_SSLMODE=disable

JWT_SECRET=super_secret_key_change_in_production
CHUNK_SIZE_MB=5
REPLICATION_FACTOR=2

GATEWAY_PORT=8000
STORAGE_NODE_PORT=9001
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef
```

### 3. Run the Cluster

Open separate terminals for each process:

**Terminal 1 — Gateway:**
```bash
go run ./cmd/gateway/
```

**Terminal 2 — Storage Node A:**
```bash
STORAGE_DATA_DIR=./data/nodeA STORAGE_NODE_PORT=9001 go run ./cmd/storagenode/
```

**Terminal 3 — Storage Node B (optional, for replication):**
```bash
STORAGE_DATA_DIR=./data/nodeB STORAGE_NODE_PORT=9002 go run ./cmd/storagenode/
```

Each storage node automatically registers itself with the Gateway on startup. The Gateway's Hash Ring will update within 10 seconds.

### 4. Access the Dashboard

**[http://localhost:8000](http://localhost:8000)**

---

## 📚 Internal Documentation

- [`internal.md`](./internal.md) — Directory maps, request flows, and component breakdown.
- [`learning.md`](./learning.md) — Concept explanations (gRPC, Consistent Hashing, AES-GCM, Connection Pooling, JWTs, etc.).
- [`PLAN.md`](./PLAN.md) — The full development roadmap.
