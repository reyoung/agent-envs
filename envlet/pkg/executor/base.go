package executor

import (
	"context"

	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

type Executor interface {
	Execute(ctx context.Context, spec *jobqueue.JobSpec) (*ExecuteResult, error)
	Close() error
}

type FileContent struct {
	Filename string `json:"filename"`
	Content  []byte `json:"content"`
}

type ExecuteResult struct {
	Stdout   []byte        `json:"stdout,omitempty"`
	Stderr   []byte        `json:"stderr,omitempty"`
	ExitCode int           `json:"exit_code"`
	Files    []FileContent `json:"files,omitempty"`
}
