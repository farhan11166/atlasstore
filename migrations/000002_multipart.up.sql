CREATE TABLE multipart_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE multipart_chunks (
    upload_id UUID NOT NULL REFERENCES multipart_uploads(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    hash TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    node_address TEXT NOT NULL,
    PRIMARY KEY (upload_id, chunk_index)
);