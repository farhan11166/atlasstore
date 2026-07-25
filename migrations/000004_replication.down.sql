ALTER TABLE chunks ADD COLUMN node_address TEXT;
ALTER TABLE multipart_chunks ADD COLUMN node_address TEXT;
UPDATE chunks c SET node_address = (
    SELECT cl.node_address FROM chunk_locations cl WHERE cl.chunk_id = c.id LIMIT 1
);
UPDATE multipart_chunks mc SET node_address = (
    SELECT mcl.node_address FROM multipart_chunk_locations mcl WHERE mcl.upload_id = mc.upload_id AND mcl.chunk_index = mc.chunk_index LIMIT 1
);
DROP TABLE chunk_locations;
DROP TABLE multipart_chunk_locations;