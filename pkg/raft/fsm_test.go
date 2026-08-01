package raft

import (
	"bytes"
	"io"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/raft"
)

// mockSnapshotSink implements raft.SnapshotSink for testing.
type mockSnapshotSink struct {
	buf *bytes.Buffer
}

func (m *mockSnapshotSink) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}
func (m *mockSnapshotSink) Close() error  { return nil }
func (m *mockSnapshotSink) ID() string    { return "test" }
func (m *mockSnapshotSink) Cancel() error { return nil }

func TestClusterFSM(t *testing.T) {
	fsm := NewClusterFSM()

	// 1. Test Apply (Join)
	cmdJoin := Command{Type: CmdNodeJoin, Address: "localhost:9001"}
	data, _ := cmdJoin.Encode()
	fsm.Apply(&raft.Log{Data: data})

	cmdJoin2 := Command{Type: CmdNodeJoin, Address: "localhost:9002"}
	data2, _ := cmdJoin2.Encode()
	fsm.Apply(&raft.Log{Data: data2})

	nodes := fsm.GetNodes()
	sort.Strings(nodes)
	expected := []string{"localhost:9001", "localhost:9002"}
	if !reflect.DeepEqual(nodes, expected) {
		t.Fatalf("expected nodes %v, got %v", expected, nodes)
	}

	// 2. Test Snapshot
	snap, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	sink := &mockSnapshotSink{buf: &bytes.Buffer{}}
	err = snap.Persist(sink)
	if err != nil {
		t.Fatalf("Persist failed: %v", err)
	}
	snapshotBytes := sink.buf.Bytes()

	// 3. Test Apply (Leave) - we apply to original FSM after snapshot
	cmdLeave := Command{Type: CmdNodeLeave, Address: "localhost:9001"}
	data3, _ := cmdLeave.Encode()
	fsm.Apply(&raft.Log{Data: data3})

	nodesAfterLeave := fsm.GetNodes()
	if len(nodesAfterLeave) != 1 || nodesAfterLeave[0] != "localhost:9002" {
		t.Fatalf("expected only node 9002, got %v", nodesAfterLeave)
	}

	// 4. Test Restore (rebuild from snapshot, it should have 9001 and 9002 again)
	fsm2 := NewClusterFSM()
	err = fsm2.Restore(io.NopCloser(bytes.NewReader(snapshotBytes)))
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	restoredNodes := fsm2.GetNodes()
	sort.Strings(restoredNodes)
	if !reflect.DeepEqual(restoredNodes, expected) {
		t.Fatalf("expected restored nodes %v, got %v", expected, restoredNodes)
	}
}
