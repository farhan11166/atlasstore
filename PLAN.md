# AtlasStore - Development Roadmap & Weekly Plan

> **AtlasStore** is a distributed object storage platform written in Go. The goal is to build a production-grade storage system while learning distributed systems, storage engines, networking, and cloud-native infrastructure.

---

# Vision & Goals

AtlasStore is **not** a Dropbox clone. It is a distributed object storage system inspired by Amazon S3, MinIO, and GFS. The focus is on learning how distributed storage systems work internally.

**Core Features**: Object storage, Chunk-based file storage, Distributed storage nodes, Replication, Fault tolerance.  
**Tech Stack**: Go, PostgreSQL, REST/gRPC, Docker, Prometheus.

---

# Detailed Implementation Plan (Tickbox Approach)

## Phase 1 — MVP (Resume Ready) ✅

_Target: Weeks 1-4_

### Week 1: Project Foundation & Metadata Layer ✅

- [x] **1.1 Project Initialization & Infrastructure**
  - [x] Initialize Go module (`go mod init github.com/farhan/atlasstore`).
  - [x] Define standard Go project layout (`cmd/`, `internal/`, `pkg/`, `api/`).
  - [x] Create a `docker-compose.yml` (PostgreSQL on port 5433).
  - [x] Set up configuration management (`internal/config/config.go` reads from `.env`).
- [x] **1.2 Metadata Layer (PostgreSQL)**
  - [x] Define database schemas (`users`, `objects`, `chunks`) in `migrations/000001_init_schema.up.sql`.
  - [x] Set up migration scripts using `golang-migrate` (`internal/db/migrate.go`).
  - [x] Implement DB connection layer (`internal/db/db.go`) + user repository (`internal/db/user_repo.go`).

### Week 2: Auth & Storage Nodes ✅

- [x] **1.3 Authentication**
  - [x] Implement User Registration REST endpoint `POST /auth/register` (`internal/auth/handler.go`).
  - [x] Implement User Login REST endpoint with JWT generation `POST /auth/login` (`internal/auth/handler.go`).
  - [x] Create JWT validation middleware (`internal/auth/middleware.go`).
- [x] **1.4 Storage Nodes (Data Plane)**
  - [x] Implement local disk storage logic (`internal/storage/disk.go`).
  - [x] Create gRPC server for storage node (`cmd/storagenode/main.go`).
  - [x] Implement `SaveChunk` — saves chunk to disk.
  - [x] Implement `GetChunk` — streams chunk from disk.
  - [x] Implement `DeleteChunk` — removes chunk from disk.

### Week 3: API Gateway (Control Plane) ✅

- [x] **1.5 API Gateway Logic**
  - [x] `POST /objects` — reads body, splits into chunks, SHA-256 hashes each, POSTs to storage node, saves metadata to DB.
  - [x] `GET /objects/{id}` — fetches chunk rows from DB, pulls bytes from storage node in order, streams reassembled file to client.
  - [x] `DELETE /objects/{id}` — deletes DB row (cascades to chunks), signals storage node to remove chunk files.
  - [x] `GET /objects` — lists all objects owned by the authenticated user.
  - [x] JWT middleware wired — all object routes protected by `auth.RequireAuth`.

### Week 4: Dashboard & Wrap-Up ✅

- [x] **1.6 Simple Web Dashboard**
  - [x] Create a vanilla HTML/JS/CSS frontend.
  - [x] Implement file upload UI with a progress indicator.
  - [x] Implement a file list with download and delete buttons.
- [x] **1.7 MVP Finalization**
  - [x] Write a comprehensive `README.md` with setup/run instructions.
  - [x] Create a system architecture diagram (in README/internal).

---

## Phase 2 — Storage Engine Enhancements ✅

- [x] Implement Chunk Checksums (SHA-256) to verify data integrity upon download.
- [x] Implement parallel chunk uploading/downloading from/to storage nodes.
- [x] Enhance large file support (multipart uploads from the client via `/multipart` endpoints).

## Phase 3 — Distributed Storage Core ✅

- [x] Build a Storage Node Registration mechanism (nodes announce themselves on startup via `POST /nodes/register`).
- [x] Implement Heartbeats — background health checker pings nodes via gRPC every 10s, updates `is_active` in PostgreSQL.
- [x] Update chunk placement logic to only select _healthy_ nodes.

## Phase 4 — Replication ✅

- [x] Update Gateway upload logic to write each chunk to N nodes (configurable `REPLICATION_FACTOR`).
- [x] Update DB schema to track multiple locations per chunk (`chunk_locations` table).
- [x] Update download logic to fallback to a secondary node if the primary is unreachable.
- [x] Background repair worker to detect and fix under-replicated chunks.

---

## Phase 5 — gRPC Migration ✅

- [x] Define Protocol Buffers (`pkg/pb/storage.proto`) for inter-node communication.
- [x] Generate Go code (`protoc`) for the gRPC Server and Client.
- [x] Update Storage Node (`internal/storage/disk.go`) to implement `pb.StorageNodeServer` instead of HTTP handlers.
- [x] Update Gateway (`internal/api/storage_client.go`) to use `grpc.Dial` with connection caching and `pb.NewStorageNodeClient`.
- [x] Transition Storage Node servers to serve gRPC over raw TCP.
- [x] Migrate Health Checker from HTTP polling to gRPC `Health()` RPC.

---

## Phase 6 — Consistent Hashing & Rebalancing ✅ ← This Week

- [x] **6.1 The Hash Ring Structure**
  - [x] Implement `HashRing` in `pkg/ring/ring.go` — maps node addresses onto a `uint32` hash space using SHA-256.
  - [x] Implement Virtual Nodes (50 vNodes per physical node) for even data distribution.
  - [x] Thread-safe `AddNode`, `RemoveNode`, `GetNodes`, `IsEmpty` methods using `sync.RWMutex`.
- [x] **6.2 Dynamic Gateway Routing**
  - [x] Create `RingManager` (`internal/api/ring_manager.go`) — a background goroutine that polls PostgreSQL every 10s and rebuilds the ring from active nodes.
  - [x] Update Gateway upload/download to use `RingManager.Ring.GetNodes(chunkHash, replicationFactor)` for deterministic placement.
  - [x] Multiple Gateway instances stay in sync via PostgreSQL (no distributed state needed yet).
- [x] **6.3 Node Rebalancing Worker**
  - [x] `StartRebalancer` in `internal/api/rebalancer.go` — runs every 60s.
  - [x] Fetches all chunks from PostgreSQL, compares current node placement vs. expected ring placement.
  - [x] If a chunk is misplaced: downloads it from its current node, uploads to the expected node via gRPC, updates PostgreSQL, and deletes from the old node.

---

## Phase 7+ — Future Horizons

### Phase 7: Consensus / Raft for Cluster State Management
- [ ] Allow Storage Nodes to form their own cluster and elect a leader.
- [ ] Nodes agree on cluster membership without relying on PostgreSQL as the single source of truth.
- [ ] Implement `etcd`-style distributed key-value store for cluster configuration.
- [ ] If PostgreSQL goes down, the cluster continues to function.

### Phase 8-10: Fault Tolerance & Production Features
- [x] Encryption At Rest (AES-GCM 256-bit) — chunks encrypted before hitting the wire.
- [ ] Data Compression (snappy or zstd) before encryption.
- [ ] Circuit Breaker pattern for node communication.
- [ ] Graceful degradation when quorum is lost.

### Phase 11-12: Observability & Load Testing
- [ ] Prometheus metrics endpoint on Gateway (request rates, rebalancing operations, node counts).
- [ ] Grafana dashboard for cluster health visualization.
- [ ] `k6` or `vegeta` load testing suite.
- [ ] Distributed tracing with OpenTelemetry.
