package idempotency

import (
	"context"
	"sync"

	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/idempotency"
)

// Store is an in-memory implementation of idempotency.Store.
// It is safe for concurrent use.
type Store struct {
	mu sync.RWMutex
	m  map[idempotency.Fingerprint]idempotency.Record
}

func NewStore() *Store {
	return &Store{
		m: make(map[idempotency.Fingerprint]idempotency.Record),
	}
}

func (s *Store) Get(ctx context.Context, fp idempotency.Fingerprint) (idempotency.Record, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.m[fp]
	return rec, ok, nil
}

func (s *Store) Put(ctx context.Context, fp idempotency.Fingerprint, rec idempotency.Record) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[fp] = rec
	return nil
}
