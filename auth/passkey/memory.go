package passkey

import (
	"context"
	"encoding/json/v2"
	"sync"
	"time"
)

// MemoryFlowStore stores single-use flows in one process without background goroutines.
// MemoryFlowStore 在单进程内存储一次性流程，不启动后台 goroutine。
type MemoryFlowStore struct {
	mu       sync.Mutex
	capacity int
	flows    map[string]memoryFlow
}

type memoryFlow struct {
	data    []byte
	expires time.Time
}

// NewMemoryFlowStore constructs a bounded store. Non-positive capacity panics.
// NewMemoryFlowStore 构造有容量上限的存储。capacity 非正数时 panic。
func NewMemoryFlowStore(capacity int) *MemoryFlowStore {
	if capacity <= 0 {
		panic("passkey: flow capacity must be positive")
	}
	return &MemoryFlowStore{capacity: capacity, flows: make(map[string]memoryFlow)}
}

// Create saves an unexpired flow without replacing an existing ID.
// Create 保存未过期流程，不替换已有 ID。
func (s *MemoryFlowStore) Create(ctx context.Context, id string, flow Flow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(flow)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !flow.Session.Expires.After(now) {
		return ErrRejected
	}
	if old, ok := s.flows[id]; ok && old.expires.After(now) {
		return ErrConflict
	}
	if len(s.flows) >= s.capacity {
		for key, old := range s.flows {
			if !old.expires.After(now) {
				delete(s.flows, key)
			}
		}
		if len(s.flows) >= s.capacity {
			return ErrCapacity
		}
	}
	s.flows[id] = memoryFlow{data: data, expires: flow.Session.Expires}
	return nil
}

// Consume atomically removes a flow, including expired flows.
// Consume 原子移除流程，包括已过期流程。
func (s *MemoryFlowStore) Consume(ctx context.Context, id string) (Flow, bool, error) {
	if err := ctx.Err(); err != nil {
		return Flow{}, false, err
	}
	s.mu.Lock()
	entry, ok := s.flows[id]
	delete(s.flows, id)
	s.mu.Unlock()
	if !ok || !entry.expires.After(time.Now()) {
		return Flow{}, false, nil
	}
	var flow Flow
	if err := json.Unmarshal(entry.data, &flow); err != nil {
		return Flow{}, false, err
	}
	return flow, true, nil
}
