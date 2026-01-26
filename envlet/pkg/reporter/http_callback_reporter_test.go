package reporter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reyoung/agent-envs/envlet/pkg/executor"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

func TestHTTPCallbackReporterSuccess(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected content type %q", ct)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		received = body
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	reporter := NewHTTPCallbackReporter(server.Client())
	t.Cleanup(func() { reporter.Close() })

	spec := &jobqueue.JobSpec{
		ID:          "job-123",
		CallbackURL: server.URL,
	}
	result := &executor.ExecuteResult{
		Stdout:   []byte("hello"),
		Stderr:   []byte(""),
		ExitCode: 0,
	}
	execErr := errors.New("boom")

	if err := reporter.Report(context.Background(), spec, result, execErr); err != nil {
		t.Fatalf("report failed: %v", err)
	}

	var payload ReportPayload
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if payload.ID != spec.ID {
		t.Fatalf("expected id %q, got %q", spec.ID, payload.ID)
	}
	if payload.Exec.Result == nil {
		t.Fatalf("expected result payload")
	}
	if got := string(payload.Exec.Result.Stdout); got != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", got)
	}
	if payload.Exec.Error != execErr.Error() {
		t.Fatalf("expected exec error %q, got %q", execErr.Error(), payload.Exec.Error)
	}
}

func TestHTTPCallbackReporterHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "callback failed", http.StatusBadRequest)
	}))
	defer server.Close()

	reporter := NewHTTPCallbackReporter(server.Client())
	t.Cleanup(func() { reporter.Close() })

	spec := &jobqueue.JobSpec{
		ID:          "job-err",
		CallbackURL: server.URL,
	}

	err := reporter.Report(context.Background(), spec, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "callback failed") {
		t.Fatalf("expected error mentioning callback, got %v", err)
	}
}

func TestHTTPCallbackReporterMissingCallback(t *testing.T) {
	reporter := NewHTTPCallbackReporter(nil)
	t.Cleanup(func() { reporter.Close() })

	spec := &jobqueue.JobSpec{ID: "missing"}
	err := reporter.Report(context.Background(), spec, nil, nil)
	if err == nil {
		t.Fatalf("expected error for missing callback URL")
	}
}
