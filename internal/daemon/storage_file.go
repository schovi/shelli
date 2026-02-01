package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

type FileStorage struct {
	dataDir string
	mu      sync.RWMutex
}

func NewFileStorage(dataDir string) (*FileStorage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &FileStorage{dataDir: dataDir}, nil
}

func (s *FileStorage) outputPath(session string) string {
	return filepath.Join(s.dataDir, session+".out")
}

func (s *FileStorage) metaPath(session string) string {
	return filepath.Join(s.dataDir, session+".meta")
}

func (s *FileStorage) Append(session string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.outputPath(session), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open output file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock file: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func (s *FileStorage) ReadFrom(session string, offset int64) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(s.outputPath(session))
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("open output file: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, fmt.Errorf("seek: %w", err)
	}

	data, err := os.ReadFile(s.outputPath(session))
	if err != nil {
		return nil, fmt.Errorf("read output: %w", err)
	}

	if offset >= int64(len(data)) {
		return []byte{}, nil
	}
	return data[offset:], nil
}

func (s *FileStorage) ReadAll(session string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.outputPath(session))
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("read output: %w", err)
	}
	return data, nil
}

func (s *FileStorage) Size(session string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, err := os.Stat(s.outputPath(session))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat output: %w", err)
	}
	return info.Size(), nil
}

func (s *FileStorage) Create(session string, meta *SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	outPath := s.outputPath(session)
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("session %q already exists", session)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	f.Close()

	return s.saveMetaLocked(session, meta)
}

func (s *FileStorage) Delete(session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	os.Remove(s.outputPath(session))
	os.Remove(s.metaPath(session))
	return nil
}

func (s *FileStorage) Exists(session string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, err := os.Stat(s.metaPath(session))
	return err == nil
}

func (s *FileStorage) LoadMeta(session string) (*SessionMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.metaPath(session))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session %q not found", session)
		}
		return nil, fmt.Errorf("read meta: %w", err)
	}

	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse meta: %w", err)
	}
	return &meta, nil
}

func (s *FileStorage) SaveMeta(session string, meta *SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMetaLocked(session, meta)
}

func (s *FileStorage) saveMetaLocked(session string, meta *SessionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	if err := os.WriteFile(s.metaPath(session), data, 0644); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	return nil
}

func (s *FileStorage) ListSessions() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read data dir: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".meta") {
			sessions = append(sessions, strings.TrimSuffix(name, ".meta"))
		}
	}
	return sessions, nil
}
