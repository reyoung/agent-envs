package jobqueue

import "context"

type JobSpec struct {
	CallbackURL    string   `json:"callback_url"`
	ID             string   `json:"id"`
	Binary         []byte   `json:"binary"`
	CapturePattern string   `json:"capture_pattern"`
	Args           []string `json:"args"`
}

type JobQueue interface {
	Fetch(ctx context.Context) (*JobSpec, error)
	Close() error
}
