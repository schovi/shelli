package daemon

import "time"

type SessionState string

const (
	StateRunning SessionState = "running"
	StateStopped SessionState = "stopped"
)

type SessionMeta struct {
	Name      string       `json:"name"`
	Command   string       `json:"command"`
	PID       int          `json:"pid"`
	State     SessionState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	StoppedAt *time.Time   `json:"stopped_at,omitempty"`
	ReadPos   int64        `json:"read_pos"`
}

type OutputStorage interface {
	Append(session string, data []byte) error
	ReadFrom(session string, offset int64) ([]byte, error)
	ReadAll(session string) ([]byte, error)
	Size(session string) (int64, error)

	Create(session string, meta *SessionMeta) error
	Delete(session string) error
	Exists(session string) bool

	LoadMeta(session string) (*SessionMeta, error)
	SaveMeta(session string, meta *SessionMeta) error
	ListSessions() ([]string, error)
}
