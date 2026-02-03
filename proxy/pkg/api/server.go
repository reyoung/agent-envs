package api

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
	exec_v1 "github.com/reyoung/agent-envs/proxy/pkg/api/proto/exec.v1"
	"github.com/reyoung/agent-envs/proxy/pkg/publisher"
	"github.com/reyoung/agent-envs/proxy/pkg/response_store"
)

type Server struct {
	exec_v1.UnimplementedProxyServer

	Publisher   publisher.Publisher
	Store       response_store.ResponseStore
	CallbackURL string
}

var (
	ErrEmptyQueueName = errors.New("queue name cannot be empty")
)

func (s *Server) Exec(ctx context.Context, req *exec_v1.ExecRequest) (*exec_v1.ExecResponse, error) {
	queueName := req.GetQueueName()
	if queueName == "" {
		return nil, ErrEmptyQueueName
	}
	reqID := uuid.New().String()
	w, err := s.Store.Waiter(reqID)
	if err != nil {
		return nil, err
	}
	defer w.Close()

	err = s.Publisher.Enqueue(ctx, queueName, &jobqueue.JobSpec{
		CallbackURL:    s.CallbackURL,
		ID:             reqID,
		Binary:         req.GetBinary(),
		CapturePattern: req.GetCapturePattern(),
		Args:           req.GetArgs(),
	})
	if err != nil {
		return nil, err
	}

	resp, err := w.Wait(ctx)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	var files []*exec_v1.ExecResponse_FileContent
	for _, f := range resp.Result.Files {
		files = append(files, &exec_v1.ExecResponse_FileContent{
			Path: f.Filename,
			Data: f.Content,
		})
	}

	return &exec_v1.ExecResponse{
		ExitCode: uint32(resp.Result.ExitCode),
		Stdout:   resp.Result.Stdout,
		Stderr:   resp.Result.Stderr,
		Files:    files,
	}, nil
}
