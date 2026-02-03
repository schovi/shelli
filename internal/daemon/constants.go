package daemon

import "time"

const (
	ReadBufferSize       = 4096
	KillGracePeriod      = 100 * time.Millisecond
	ClientDeadline       = 30 * time.Second
	DaemonStartTimeout   = 5 * time.Second
	DaemonPollInterval   = 100 * time.Millisecond
	DefaultMaxOutputSize = 10 * 1024 * 1024 // 10 MB

	ReadModeNew = "new"
	ReadModeAll = "all"
)
