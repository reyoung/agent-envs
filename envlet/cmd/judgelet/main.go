package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

type TestSpec struct {
	Input  []byte `json:"input"`
	Output []byte `json:"output"`
	Name   string `json:"name"`
}

type TestResult struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

var (
	flagTests   = flag.String("tests-file", "", "Path to a JSON lines file containing test specifications.")
	flagTestBin = flag.String("test-bin", "", "Path to the test binary to execute.")
	flagTimeout = flag.Duration("timeout", 10*time.Second, "Per-test timeout. Use 0 for no timeout.")
)

func main() {
	log.SetFlags(0)
	flag.Parse()

	testsPath := strings.TrimSpace(*flagTests)
	if testsPath == "" {
		log.Fatalf("--tests-file is required")
	}
	testBinary := strings.TrimSpace(*flagTestBin)
	if testBinary == "" {
		log.Fatalf("--test-bin is required")
	}

	testSpecs, err := loadTests(testsPath)
	if err != nil {
		log.Fatalf("load tests: %v", err)
	}
	if len(testSpecs) == 0 {
		log.Fatalf("no tests found in %s", testsPath)
	}

	var failed bool
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	for idx, spec := range testSpecs {
		spec.Name = strings.TrimSpace(spec.Name)
		if spec.Name == "" {
			spec.Name = fmt.Sprintf("test-%03d", idx+1)
		}
		res := runSingleTest(testBinary, spec, *flagTimeout)
		if res.Error != "" {
			failed = true
		}
		if err := enc.Encode(res); err != nil {
			log.Fatalf("encode result for %s: %v", spec.Name, err)
		}
	}

	if failed {
		os.Exit(1)
	}
}

func loadTests(path string) ([]TestSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tests file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	var specs []TestSpec
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var spec TestSpec
		if err := json.Unmarshal([]byte(text), &spec); err != nil {
			return nil, fmt.Errorf("decode test spec on line %d: %w", line, err)
		}
		specs = append(specs, spec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan tests: %w", err)
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("no tests found in %s", path)
	}
	return specs, nil
}

func runSingleTest(binaryPath string, spec TestSpec, timeout time.Duration) TestResult {
	result := TestResult{Name: spec.Name}

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Stdin = bytes.NewReader(spec.Input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
			result.Error = fmt.Sprintf("timeout after %s", timeout)
			return result
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderrMsg := strings.TrimSpace(stderr.String())
			if stderrMsg != "" {
				result.Error = fmt.Sprintf("exit code %d: %s", exitErr.ExitCode(), stderrMsg)
			} else {
				result.Error = fmt.Sprintf("exit code %d", exitErr.ExitCode())
			}
			return result
		}
		result.Error = fmt.Sprintf("failed to execute binary: %v", err)
		return result
	}

	if !bytes.Equal(stdout.Bytes(), spec.Output) {
		result.Error = formatMismatch(stdout.Bytes(), spec.Output)
	}
	return result
}

func formatMismatch(got, want []byte) string {
	return fmt.Sprintf("output mismatch: got %s (len=%d), want %s (len=%d)",
		previewBytes(got), len(got), previewBytes(want), len(want))
}

func previewBytes(b []byte) string {
	if len(b) == 0 {
		return "<empty>"
	}
	const limit = 64
	if len(b) > limit {
		return fmt.Sprintf("%q...", string(b[:limit]))
	}
	return fmt.Sprintf("%q", string(b))
}
