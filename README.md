# AtlasStore

> A distributed object storage platform built in Go.

AtlasStore is a learning-focused implementation of a distributed object storage system, heavily inspired by Amazon S3 and MinIO. It separates the Control Plane (API Gateway) from the Data Plane (Storage Nodes) to orchestrate chunked file uploads, distributed storage, and file reassembly.

---

## 🚀 Features (Phase 1 MVP)

- **Two-Server Architecture:** Decoupled Gateway (Port 8000) and Storage Node (Port 9000).
- **Chunking Engine:** Large files are streamed and split into 5MB chunks on upload.
- **SHA-256 Integrity:** Chunks are content-hashed on upload and stored deterministically.
- **Stateless Auth:** JWT-based registration and login system.
- **PostgreSQL Metadata:** Tracks users, objects, chunk indices, and storage node placement.
- **Streaming I/O:** Uploads and downloads are streamed through `io.Copy` to ensure memory safety, regardless of file size.
- **Premium Web Dashboard:** A vanilla HTML/JS/CSS dashboard to manage files with drag-and-drop support, progress bars, and file-type icons.

---

## 🏗️ Architecture

```text
       [Web Dashboard]
              │ (HTTP / JSON)
              ▼
   ┌──────────────────────┐
   │     API Gateway      │ ── (Metadata) ──► [ PostgreSQL DB ]
   │  (Control Plane)     │
   └──────────────────────┘
              │ (Internal HTTP - Chunk Distribution)
              ▼
   ┌──────────────────────┐
   │    Storage Node 1    │ ── (Raw Bytes) ──► [ Local Disk /data/node1 ]
   │    (Data Plane)      │
   └──────────────────────┘
```

## 🛠️ Tech Stack

- **Backend:** Go 1.24 (Standard Library for HTTP and Crypto)
- **Database:** PostgreSQL (via `lib/pq` and `golang-migrate`)
- **Security:** `golang.org/x/crypto/bcrypt` (Password Hashing), `github.com/golang-jwt/jwt/v5` (Auth)
- **Frontend:** Vanilla HTML, CSS, JavaScript (No frameworks)

---

## ⚙️ Getting Started

### Prerequisites
- [Go 1.24+](https://go.dev/)
- [Docker & Docker Compose](https://www.docker.com/)

### 1. Setup Database
Start the PostgreSQL container:
```bash
docker-compose up -d
```

### 2. Configure Environment
Ensure your `.env` file looks like this:
```env
DB_HOST=localhost
DB_PORT=5433
DB_USER=atlasstore
DB_PASSWORD=atlaspassword
DB_NAME=atlasstore
DB_SSLMODE=disable

JWT_SECRET=super_secret_key_change_in_production
CHUNK_SIZE_MB=5

GATEWAY_PORT=8000
STORAGE_NODE_PORT=9000
```

### 3. Run the Servers
You need to run the Gateway and the Storage Node in two separate terminal windows.

**Terminal 1 (Gateway):**
```bash
go run ./cmd/gateway/
```
*(This will automatically run database migrations and serve the frontend on port 8000).*

**Terminal 2 (Storage Node):**
```bash
STORAGE_DATA_DIR=./data/node1 go run ./cmd/storagenode/
```

### 4. Access the Dashboard
Open your browser and navigate to:
**[http://localhost:8000](http://localhost:8000)**

---

## 📚 Internal Documentation

For deep dives into *how* and *why* this system is built the way it is, check out:
- [`internal.md`](./internal.md) — Directory maps, request flows, and component breakdown.
- [`learning.md`](./learning.md) — Concept explanations (Connection Pooling, Hashing, Streaming, JWTs).
- [`PLAN.md`](./PLAN.md) — The development roadmap and upcoming distributed systems features.
