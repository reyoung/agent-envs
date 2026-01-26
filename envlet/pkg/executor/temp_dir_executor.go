package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

type tempDirExecutor struct{}

func New() (Executor, error) {
	return &tempDirExecutor{}, nil
}

func (e *tempDirExecutor) Close() error {
	return nil
}

func (e *tempDirExecutor) Execute(ctx context.Context, spec *jobqueue.JobSpec) (*ExecuteResult, error) {
	if spec == nil {
		return nil, fmt.Errorf("job spec is nil")
	}
	if len(spec.Binary) == 0 {
		return nil, fmt.Errorf("job binary is empty")
	}

	workdir, err := os.MkdirTemp("", "envlet-exec-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workdir)

	binaryPath := filepath.Join(workdir, "job-binary")
	if err := os.WriteFile(binaryPath, spec.Binary, 0o700); err != nil {
		return nil, fmt.Errorf("write binary: %w", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binaryPath, spec.Args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = workdir

	result := &ExecuteResult{}
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.Stdout = stdout.Bytes()
			result.Stderr = stderr.Bytes()
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return nil, fmt.Errorf("execute binary: %w", err)
	}

	result.Stdout = stdout.Bytes()
	result.Stderr = stderr.Bytes()
	if state := cmd.ProcessState; state != nil {
		result.ExitCode = state.ExitCode()
	}

	log.Printf("execution capture pattern %s", spec.CapturePattern)

	if spec.CapturePattern != "" {
		pattern, err := regexp.Compile(spec.CapturePattern)
		if err != nil {
			return nil, fmt.Errorf("compile capture pattern: %w", err)
		}

		// Walk through the working directory to find matching files
		err = filepath.Walk(workdir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(workdir, path)
			if err != nil {
				return err
			}
			log.Printf("checking file for capture: %s", relPath)
			if pattern.MatchString(relPath) {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				result.Files = append(result.Files, FileContent{
					Filename: relPath,
					Content:  content,
				})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("capture files: %w", err)
		}
	}

	return result, nil
}
