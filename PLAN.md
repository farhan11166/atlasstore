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
*Target: Weeks 1-4*

### Week 1: Project Foundation & Metadata Layer
- [ ] **1.1 Project Initialization & Infrastructure**
  - [ ] Initialize Go module (`go mod init github.com/yourusername/atlasstore`).
  - [ ] Define standard Go project layout (`cmd/`, `internal/`, `pkg/`, `api/`).
  - [ ] Create a `docker-compose.yml` (PostgreSQL, API Gateway, 3 Storage Nodes).
  - [ ] Set up basic configuration management (e.g., reading from `.env` or YAML).
- [ ] **1.2 Metadata Layer (PostgreSQL)**
  - [ ] Define database schemas (`users`, `objects`, `chunks`).
  - [ ] Set up database migration scripts (using a tool like `golang-migrate`).
  - [ ] Implement Go repository layer for connecting to PostgreSQL and performing CRUD operations.

### Week 2: Auth & Storage Nodes
- [ ] **1.3 Authentication**
  - [ ] Implement User Registration REST endpoint (`POST /auth/register`).
  - [ ] Implement User Login REST endpoint with JWT generation (`POST /auth/login`).
  - [ ] Create JWT validation middleware to protect API routes.
- [ ] **1.4 Storage Nodes (Data Plane)**
  - [ ] Implement local disk storage logic (saving/reading files from disk).
  - [ ] Create a basic HTTP server for each storage node.
  - [ ] Implement `POST /chunk` to receive and save a chunk.
  - [ ] Implement `GET /chunk/{hash}` to stream a chunk back.
  - [ ] Implement `DELETE /chunk/{hash}` to remove a chunk from disk.

### Week 3: API Gateway (Control Plane)
- [ ] **1.5 API Gateway Logic**
  - [ ] Implement `POST /objects` (Upload): Stream the incoming file, break it into fixed-size chunks, distribute chunks to 3 storage nodes, and save metadata.
  - [ ] Implement `GET /objects/{id}` (Download): Fetch chunk metadata, pull chunks from storage nodes, reassemble, and stream to client.
  - [ ] Implement `DELETE /objects/{id}`: Delete from DB and signal storage nodes to remove chunks.
  - [ ] Implement `GET /objects`: List user's uploaded files.

### Week 4: Dashboard & Wrap-Up
- [ ] **1.6 Simple Web Dashboard**
  - [ ] Create a vanilla HTML/JS/CSS frontend.
  - [ ] Implement file upload UI with a progress indicator.
  - [ ] Implement a file list with download and delete buttons.
- [ ] **1.7 MVP Finalization**
  - [ ] Write a comprehensive `README.md` with setup/run instructions.
  - [ ] Create a system architecture diagram.
  - [ ] Record a demo video for resume/portfolio.o

---

## Phase 2 — Storage Engine Enhancements (Week 5)
- [ ] Implement Chunk Checksums (SHA-256) to verify data integrity upon download.
- [ ] Implement parallel chunk uploading/downloading from/to storage nodes.
- [ ] Enhance large file support (handling multipart uploads from the client).

## Phase 3 — Distributed Storage Core (Week 6)
- [ ] Build a Storage Node Registration mechanism (nodes announce themselves on startup).
- [ ] Implement Heartbeats (`/health` checks) from Gateway to Storage nodes.
- [ ] Update chunk placement logic to only select *healthy* nodes.

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
