package response_store

import (
	"context"

	"github.com/reyoung/agent-envs/proxy/pkg/model"
)

type Waiter interface {
	Wait(ctx context.Context) (model.Response, error)
	Close()
}

type ResponseStore interface {
	Close()
	Waiter(id string) (Waiter, error)
	Deliver(id string, resp model.Response)
}
