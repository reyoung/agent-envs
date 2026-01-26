package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/reyoung/agent-envs/envlet/pkg/executor"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

type ReportPayload struct {
	ID   string `json:"id"`
	Exec struct {
		Result *executor.ExecuteResult `json:"result,omitempty"`
		Error  string                  `json:"error,omitempty"`
	} `json:"exec"`
}

type HTTPCallbackReporter struct {
	client *http.Client
}

func NewHTTPCallbackReporter(client *http.Client) *HTTPCallbackReporter {
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxConnsPerHost = 256
		transport.MaxIdleConns = 512
		transport.MaxIdleConnsPerHost = 256
		client = &http.Client{Transport: transport}
	}
	return &HTTPCallbackReporter{client: client}
}

func (r *HTTPCallbackReporter) Report(ctx context.Context, jobSpec *jobqueue.JobSpec,
	result *executor.ExecuteResult, execErr error) error {
	if jobSpec == nil {
		return fmt.Errorf("job spec is nil")
	}

	callbackURL := strings.TrimSpace(jobSpec.CallbackURL)
	if callbackURL == "" {
		return fmt.Errorf("job %q is missing callback URL", jobSpec.ID)
	}

	payload := ReportPayload{ID: jobSpec.ID}
	payload.Exec.Result = result
	if execErr != nil {
		payload.Exec.Error = execErr.Error()
	}

	body, err := json.Marshal(&payload)
	if err != nil {
		return fmt.Errorf("encode report payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create callback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("send callback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		msg := strings.TrimSpace(string(snippet))
		if msg != "" {
			return fmt.Errorf("callback %s failed: %s (%s)", callbackURL, resp.Status, msg)
		}
		return fmt.Errorf("callback %s failed: %s", callbackURL, resp.Status)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (r *HTTPCallbackReporter) Close() error {
	return nil
}
