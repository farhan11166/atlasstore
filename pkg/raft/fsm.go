package raft

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

type ClusterFSM struct {
	mu    sync.RWMutex
	nodes map[string]bool
}

func NewClusterFSM() *ClusterFSM {
	return &ClusterFSM{
		nodes: make(map[string]bool),
	}
}

// to commit entries
func (f *ClusterFSM) Apply(log *raft.Log) any {
	cmd, err := DecodeCommand((log.Data))
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	switch cmd.Type {
	case CmdNodeJoin:

		f.nodes[cmd.Address] = true
	case CmdNodeLeave:

		delete(f.nodes, cmd.Address)

	}
	return nil
}

func (f *ClusterFSM) GetNodes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var result []string
	for addr := range f.nodes {
		result = append(result, addr)
	}
	return result
}

func (f *ClusterFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	copy := make(map[string]bool, len(f.nodes))
	for k, v := range f.nodes {
		copy[k] = v
	}
	return &fsmSnapshot{nodes: copy}, nil
}

func (f *ClusterFSM) Restore(snapshot io.ReadCloser) error {
	defer snapshot.Close()

	var nodes map[string]bool
	if err := json.NewDecoder(snapshot).Decode(&nodes); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes = nodes
	return nil
}

type fsmSnapshot struct {
	nodes map[string]bool
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if err := json.NewEncoder(sink).Encode(s.nodes); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}
