package response_store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/reyoung/agent-envs/proxy/pkg/model"
)

type waiter struct {
	id    string
	store *responseStore
	ch    chan model.Response
}

func (w *waiter) Wait(ctx context.Context) (model.Response, error) {
	select {
	case resp := <-w.ch:
		return resp, nil
	case <-ctx.Done():
		return model.Response{}, ctx.Err()
	}
}

func (w *waiter) Close() {
	w.store.release(w.id, w.ch)
}

type responseStore struct {
	mu      sync.Mutex
	entries map[string]*storeEntry
	ttl     time.Duration
	stopCh  chan struct{}
}

type storeEntry struct {
	ch     chan model.Response
	resp   *model.Response
	expiry time.Time
}

func New() ResponseStore {
	s := &responseStore{
		entries: make(map[string]*storeEntry),
		ttl:     time.Second * 300,
		stopCh:  make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *responseStore) Close() {
	close(s.stopCh)
}

func (s *responseStore) Waiter(id string) (Waiter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry == nil {
		entry = &storeEntry{}
		s.entries[id] = entry
	}

	if entry.ch != nil {
		return nil, fmt.Errorf("waiter already registered for %s", id)
	}

	if entry.resp != nil {
		// result already available; create closed channel delivering immediately
		ch := make(chan model.Response, 1)
		ch <- *entry.resp
		delete(s.entries, id)
		return &waiter{id: id, store: s, ch: ch}, nil
	}

	entry.ch = make(chan model.Response, 1)
	entry.expiry = time.Time{}
	return &waiter{id: id, store: s, ch: entry.ch}, nil
}

func (s *responseStore) Deliver(id string, resp model.Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry == nil {
		entry = &storeEntry{}
		s.entries[id] = entry
	}

	if entry.ch != nil {
		select {
		case entry.ch <- resp:
		default:
		}
		entry.ch = nil
		delete(s.entries, id)
		return
	}

	respCopy := resp
	entry.resp = &respCopy
	entry.expiry = time.Now().Add(s.ttl)
}

func (s *responseStore) release(id string, ch chan model.Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry == nil {
		return
	}

	if entry.ch == ch {
		entry.ch = nil
		if entry.resp == nil {
			entry.expiry = time.Now().Add(s.ttl)
		}
		if entry.resp == nil {
			// nothing to keep
			delete(s.entries, id)
		}
	}
}

func (s *responseStore) cleanupLoop() {
	interval := s.ttl / 2
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for id, entry := range s.entries {
				if entry.ch != nil {
					continue
				}
				if entry.resp == nil {
					delete(s.entries, id)
					continue
				}
				if !entry.expiry.IsZero() && now.After(entry.expiry) {
					delete(s.entries, id)
				}
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}
