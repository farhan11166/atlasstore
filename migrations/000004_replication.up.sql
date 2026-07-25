CREATE TABLE chunk_locations(
    chunk_id UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    node_address TEXT NOT NULL,
    PRIMARY KEY (chunk_id, node_address)
);

CREATE TABLE multipart_chunk_locations (
    upload_id UUID NOT NULL,
    chunk_index INT NOT NULL,
    node_address TEXT NOT NULL,
    FOREIGN KEY (upload_id, chunk_index) REFERENCES multipart_chunks(upload_id, chunk_index) ON DELETE CASCADE,
    PRIMARY KEY (upload_id, chunk_index, node_address)
);

INSERT INTO chunk_locations (chunk_id, node_address) SELECT id, node_address FROM chunks;
INSERT INTO multipart_chunk_locations (upload_id, chunk_index, node_address) SELECT upload_id, chunk_index, node_address FROM multipart_chunks;

ALTER TABLE chunks DROP COLUMN node_address;
ALTER TABLE multipart_chunks DROP COLUMN node_address;