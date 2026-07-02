CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE TABLE IF NOT EXISTS users(
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    created_at TIMESTAMPZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS objects(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()

);

CREATE TABLE IF NOT EXISTS chunks(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 object_id UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
 chunk_index INT NOT NULL, -- 0 based order of the chunksss
 hash TEXT NOT NULL, ---SHA 256 crypto stuff :)
 size BIGINT NOT NULL,
 node_address TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

 UNIQUE(object_id,chunk_index)
);