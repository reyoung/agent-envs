package model

import (
	"fmt"
	"strings"

	"github.com/reyoung/agent-envs/envlet/pkg/executor"
)

type Request struct {
	QueueName      string   `json:"queue_name"`
	Binary         []byte   `json:"binary"`
	CapturePattern string   `json:"capture_pattern,omitempty"`
	Args           []string `json:"args,omitempty"`
}

type Response struct {
	Result *executor.ExecuteResult `json:"result,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

func (req *Request) Normalize() error {
	req.QueueName = strings.TrimSpace(req.QueueName)
	if req.QueueName == "" {
		return fmt.Errorf("queue_name is required")
	}
	if len(req.Binary) == 0 {
		return fmt.Errorf("binary is required")
	}
	return nil
}
