package publisher

import (
	"context"

	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

type Publisher interface {
	Close() error
	Enqueue(ctx context.Context, queueName string, spec *jobqueue.JobSpec) error
}
