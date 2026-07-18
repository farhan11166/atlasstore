# AtlasStore - Development Roadmap & Weekly Plan

> **AtlasStore** is a distributed object storage platform written in Go. The goal is to build a production-grade storage system while learning distributed systems, storage engines, networking, and cloud-native infrastructure.

---

# Vision & Goals

AtlasStore is **not** a Dropbox clone. It is a distributed object storage system inspired by Amazon S3, MinIO, and GFS. The focus is on learning how distributed storage systems work internally.

**Core Features**: Object storage, Chunk-based file storage, Distributed storage nodes, Replication, Fault tolerance.
**Tech Stack**: Go, PostgreSQL, Redis, REST/gRPC, Docker, Prometheus.

---

# Detailed Implementation Plan (Tickbox Approach)

## Phase 1 — MVP (Resume Ready)

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
  - [x] Create JWT validation middleware (`internal/auth/middleware.go`) — wired in Week 3 when object routes are added.
- [x] **1.4 Storage Nodes (Data Plane)**
  - [x] Implement local disk storage logic (`internal/storage/disk.go`).
  - [x] Create HTTP server for storage node (`cmd/storagenode/main.go`).
  - [x] Implement `POST /chunk` — saves chunk to disk.
  - [x] Implement `GET /chunk/{hash}` — streams chunk from disk.
  - [x] Implement `DELETE /chunk/{hash}` — removes chunk from disk.

### Week 3: API Gateway (Control Plane) ✅

- [x] **1.5 API Gateway Logic**
  - [x] `POST /objects` — reads body, splits into chunks, SHA-256 hashes each, POSTs to storage node, saves metadata to DB (`internal/api/object_handler.go`).
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
  - [x] Record a demo video for resume/portfolio.

---

## Phase 2 — Storage Engine Enhancements (Week 5)

- [x] Implement Chunk Checksums (SHA-256) to verify data integrity upon download.
- [x] Implement parallel chunk uploading/downloading from/to storage nodes.
- [ ] Enhance large file support (handling multipart uploads from the client).

## Phase 3 — Distributed Storage Core (Week 6)

- [ ] Build a Storage Node Registration mechanism (nodes announce themselves on startup).
- [ ] Implement Heartbeats (`/health` checks) from Gateway to Storage nodes.
- [ ] Update chunk placement logic to only select _healthy_ nodes.

## Phase 4 — Replication (Weeks 7-8)

- [ ] Update Gateway upload logic to write each chunk to N nodes (e.g., Replication Factor = 2).
- [ ] Update DB schema to track multiple locations per chunk.
- [ ] Update download logic to fallback to a secondary node if the primary is unreachable.
- [ ] Create a background repair worker to detect under-replicated chunks.

## Phase 5+ — Advanced Distributed Systems (Weeks 9+)

- [ ] **Phase 5**: Migration from REST to gRPC for internal node communication.
- [ ] **Phase 6**: Consistent Hashing ring for dynamic node addition.
- [ ] **Phase 7**: Consensus / Raft for Cluster state management.
- [ ] **Phase 8-10**: Fault Tolerance, Production Features (Encryption, Compressions), Cloud Native (K8s).
- [ ] **Phase 11-12**: Observability (Prometheus/Grafana) & Load Testing.
