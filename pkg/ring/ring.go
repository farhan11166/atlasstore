package ring

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// HashRing manages the consistent hashing ring for storage nodes.
type HashRing struct {
	mu       sync.RWMutex
	vNodes   int               // number of virtual nodes per physical node
	keys     []uint32          // sorted hashes of virtual nodes
	ring     map[uint32]string // maps hash to physical node address
	nodesMap map[string]bool   // tracks if a physical node is currently in the ring
}

// New creates a new HashRing with the specified number of virtual nodes.
func New(vNodes int) *HashRing {
	return &HashRing{
		vNodes:   vNodes,
		ring:     make(map[uint32]string),
		nodesMap: make(map[string]bool),
	}
}

func hashKey(key string) uint32 {
	h := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(h[:4])
}

// AddNode adds a physical node to the ring.
func (h *HashRing) AddNode(node string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.nodesMap[node] {
		return // already in ring
	}
	h.nodesMap[node] = true

	for i := 0; i < h.vNodes; i++ {
		vNodeKey := fmt.Sprintf("%s-%d", node, i)
		hash := hashKey(vNodeKey)
		h.keys = append(h.keys, hash)
		h.ring[hash] = node
	}
	
	sort.Slice(h.keys, func(i, j int) bool {
		return h.keys[i] < h.keys[j]
	})
}

// RemoveNode removes a physical node from the ring.
func (h *HashRing) RemoveNode(node string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.nodesMap[node] {
		return
	}
	delete(h.nodesMap, node)

	var newKeys []uint32
	for _, hash := range h.keys {
		if h.ring[hash] == node {
			delete(h.ring, hash)
		} else {
			newKeys = append(newKeys, hash)
		}
	}
	h.keys = newKeys
}

// IsEmpty returns true if there are no physical nodes in the ring.
func (h *HashRing) IsEmpty() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodesMap) == 0
}

// GetNodes returns 'count' distinct nodes for the given chunk hash.
func (h *HashRing) GetNodes(chunkHash string, count int) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.nodesMap) == 0 {
		return nil
	}

	hash := hashKey(chunkHash)
	
	idx := sort.Search(len(h.keys), func(i int) bool {
		return h.keys[i] >= hash
	})

	if idx == len(h.keys) {
		idx = 0
	}

	var selectedNodes []string
	seen := make(map[string]bool)

	for i := 0; i < len(h.keys); i++ {
		nodeIndex := (idx + i) % len(h.keys)
		nodeAddress := h.ring[h.keys[nodeIndex]]
		
		if !seen[nodeAddress] {
			seen[nodeAddress] = true
			selectedNodes = append(selectedNodes, nodeAddress)
		}
		
		if len(selectedNodes) == count {
			break
		}
	}

	return selectedNodes
}