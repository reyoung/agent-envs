package executor

import (
	"context"
	"testing"

	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

func TestTempDirExecutorSuccess(t *testing.T) {
	exec, err := New()
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	spec := &jobqueue.JobSpec{
		ID:          "job-1",
		CallbackURL: "http://example.com",
		Binary: []byte(`#!/bin/sh
printf hello`),
	}

	result, err := exec.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("execute job: %v", err)
	}

	if got, want := string(result.Stdout), "hello"; got != want {
		t.Fatalf("stdout mismatch: got %q want %q", got, want)
	}
	if len(result.Stderr) != 0 {
		t.Fatalf("stderr should be empty, got %q", string(result.Stderr))
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestTempDirExecutorNonZeroExit(t *testing.T) {
	exec, err := New()
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	spec := &jobqueue.JobSpec{
		ID:          "job-2",
		CallbackURL: "http://example.com",
		Binary: []byte(`#!/bin/sh
printf 'error\n' >&2
exit 3`),
	}

	result, err := exec.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("execute job: %v", err)
	}

	if got, want := string(result.Stderr), "error\n"; got != want {
		t.Fatalf("stderr mismatch: got %q want %q", got, want)
	}
	if result.ExitCode != 3 {
		t.Fatalf("expected exit code 3, got %d", result.ExitCode)
	}
}
