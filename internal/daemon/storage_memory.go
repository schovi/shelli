package daemon

import (
	"fmt"
	"sync"
)

type MemoryStorage struct {
	mu            sync.RWMutex
	outputs       map[string][]byte
	metas         map[string]*SessionMeta
	maxOutputSize int
}

func NewMemoryStorage(maxOutputSize int) *MemoryStorage {
	return &MemoryStorage{
		outputs:       make(map[string][]byte),
		metas:         make(map[string]*SessionMeta),
		maxOutputSize: maxOutputSize,
	}
}

func (s *MemoryStorage) Append(session string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.metas[session]; !exists {
		return fmt.Errorf("session %q not found", session)
	}

	s.outputs[session] = append(s.outputs[session], data...)

	if s.maxOutputSize > 0 && len(s.outputs[session]) > s.maxOutputSize {
		excess := len(s.outputs[session]) - s.maxOutputSize
		s.outputs[session] = s.outputs[session][excess:]
		if meta, ok := s.metas[session]; ok {
			if meta.ReadPos > 0 {
				meta.ReadPos = max(0, meta.ReadPos-int64(excess))
			}
			for k, v := range meta.Cursors {
				meta.Cursors[k] = max(0, v-int64(excess))
			}
		}
	}

	return nil
}

func (s *MemoryStorage) ReadFrom(session string, offset int64) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	output, exists := s.outputs[session]
	if !exists {
		return nil, fmt.Errorf("session %q not found", session)
	}

	if offset >= int64(len(output)) {
		return []byte{}, nil
	}
	return append([]byte{}, output[offset:]...), nil
}

func (s *MemoryStorage) ReadAll(session string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	output, exists := s.outputs[session]
	if !exists {
		return nil, fmt.Errorf("session %q not found", session)
	}
	return append([]byte{}, output...), nil
}

func (s *MemoryStorage) Size(session string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	output, exists := s.outputs[session]
	if !exists {
		return 0, fmt.Errorf("session %q not found", session)
	}
	return int64(len(output)), nil
}

func (s *MemoryStorage) Clear(session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.metas[session]; !exists {
		return fmt.Errorf("session %q not found", session)
	}

	s.outputs[session] = []byte{}
	if meta, ok := s.metas[session]; ok {
		meta.ReadPos = 0
		meta.Cursors = nil
	}
	return nil
}

func (s *MemoryStorage) Create(session string, meta *SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.metas[session]; exists {
		return fmt.Errorf("session %q already exists", session)
	}

	s.outputs[session] = []byte{}
	s.metas[session] = meta
	return nil
}

func (s *MemoryStorage) Delete(session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.outputs, session)
	delete(s.metas, session)
	return nil
}

func (s *MemoryStorage) Exists(session string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.metas[session]
	return exists
}

func (s *MemoryStorage) LoadMeta(session string) (*SessionMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	meta, exists := s.metas[session]
	if !exists {
		return nil, fmt.Errorf("session %q not found", session)
	}
	copied := *meta
	if meta.Cursors != nil {
		copied.Cursors = make(map[string]int64, len(meta.Cursors))
		for k, v := range meta.Cursors {
			copied.Cursors[k] = v
		}
	}
	if meta.StoppedAt != nil {
		t := *meta.StoppedAt
		copied.StoppedAt = &t
	}
	return &copied, nil
}

func (s *MemoryStorage) SaveMeta(session string, meta *SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.metas[session]; !exists {
		return fmt.Errorf("session %q not found", session)
	}
	s.metas[session] = meta
	return nil
}

func (s *MemoryStorage) UpdateMeta(session string, fn func(meta *SessionMeta)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, exists := s.metas[session]
	if !exists {
		return fmt.Errorf("session %q not found", session)
	}
	fn(meta)
	return nil
}

func (s *MemoryStorage) ListSessions() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]string, 0, len(s.metas))
	for name := range s.metas {
		sessions = append(sessions, name)
	}
	return sessions, nil
}
