package reporter

import (
	"context"

	"github.com/reyoung/agent-envs/envlet/pkg/executor"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

type Reporter interface {
	Report(ctx context.Context, jobSpec *jobqueue.JobSpec,
		result *executor.ExecuteResult, execErr error) error

	Close() error
}
