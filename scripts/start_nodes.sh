#!/bin/bash
echo "Starting Storage Node 1 (Leader)..."
STORAGE_NODE_PORT=9001 STORAGE_DATA_DIR=./data/node1 RAFT_DATA_DIR=./raft/node1 go run cmd/storagenode/main.go &
sleep 2

echo "Starting Storage Node 2..."
STORAGE_NODE_PORT=9002 STORAGE_DATA_DIR=./data/node2 RAFT_DATA_DIR=./raft/node2 RAFT_PEERS=localhost:29001 go run cmd/storagenode/main.go &
sleep 1

echo "Starting Storage Node 3..."
STORAGE_NODE_PORT=9003 STORAGE_DATA_DIR=./data/node3 RAFT_DATA_DIR=./raft/node3 RAFT_PEERS=localhost:29001 go run cmd/storagenode/main.go &

echo "All 3 nodes are starting up! Press Ctrl+C to kill them all when you are done."
wait
