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
	Input           []byte        `json:"input"`
	Output          []byte        `json:"output"`
	Name            string        `json:"name"`
	MemoryLimitInMB int           `json:"memory_limit_mb,omitempty"`
	TimeLimit       time.Duration `json:"time_limit,omitempty"`
}

type TestResult struct {
	Name    string        `json:"name"`
	Error   string        `json:"error,omitempty"`
	Details *resultOutput `json:"details,omitempty"`
}

var (
	flagTests   = flag.String("tests-file", "", "Path to a JSON lines file containing test specifications.")
	flagTestBin = flag.String("test-bin", "", "Path to the test binary to execute.")
	flagTimeout = flag.Duration("timeout", 10*time.Second, "Per-test timeout. Use 0 for no timeout.")
	flagRunProg = flag.String("runprog-bin", "", "Path to the runprog binary.")
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stderr)
	flag.Parse()

	testsPath := strings.TrimSpace(*flagTests)
	if testsPath == "" {
		log.Fatalf("--tests-file is required")
	}
	testBinary := strings.TrimSpace(*flagTestBin)
	if testBinary == "" {
		log.Fatalf("--test-bin is required")
	}
	if *flagRunProg == "" {
		log.Fatalf("--runprog-bin is required")
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

// Status defines uoj/run_program constants
type Status int

// UOJ run_program constants
const (
	StatusNormal  Status = iota // 0
	StatusInvalid               // 1
	StatusRE                    // 2
	StatusMLE                   // 3
	StatusTLE                   // 4
	StatusOLE                   // 5
	StatusBan                   // 6
	StatusFatal                 // 7
)
const RESULT_OUTPUT_FILE = "result.out"

func statusToString(s Status) string {
	switch s {
	case StatusNormal:
		return "Normal"
	case StatusInvalid:
		return "Invalid"
	case StatusRE:
		return "Runtime Error"
	case StatusMLE:
		return "Memory Limit Exceeded"
	case StatusTLE:
		return "Time Limit Exceeded"
	case StatusOLE:
		return "Output Limit Exceeded"
	case StatusBan:
		return "Disallowed Syscall"
	case StatusFatal:
		return "Fatal Error"
	default:
		return "Unknown"
	}
}

type resultOutput struct {
	TimeMs   int    `json:"time_ms"`
	MemoryKB uint64 `json:"memory_kb"`
	ExitCode int    `json:"exit_code"`
	Status   string `json:"status"`
}

func parseResultOutput() (*resultOutput, error) {
	file, err := os.Open(RESULT_OUTPUT_FILE)
	if err != nil {
		return nil, fmt.Errorf("open result file: %w", err)
	}
	defer file.Close()
	var ro resultOutput
	var status Status
	_, err = fmt.Fscanf(file, "%d %d %d %d", &status, &ro.TimeMs, &ro.MemoryKB, &ro.ExitCode)
	if err != nil {
		return nil, fmt.Errorf("parse result file: %w", err)
	}
	ro.Status = statusToString(status)
	return &ro, nil
}

func runSingleTest(binaryPath string, spec TestSpec, timeout time.Duration) TestResult {
	log.Printf("Running test %s", spec.Name)
	result := TestResult{Name: spec.Name}

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if spec.MemoryLimitInMB == 0 {
		spec.MemoryLimitInMB = 256
	}
	if spec.TimeLimit == 0 {
		spec.TimeLimit = time.Second
	}

	cmd := exec.CommandContext(ctx, *flagRunProg,
		"-ml", fmt.Sprintf("%d", spec.MemoryLimitInMB),
		"-tl", spec.TimeLimit.String(),
		"-res", RESULT_OUTPUT_FILE,
		"-runner", "container",
		"-unsafe",
		"-cgroup",
		"--bind-pwd",
		binaryPath,
	)
	log.Printf("Executing command: %s", strings.Join(cmd.Args, " "))
	cmd.Stdin = bytes.NewReader(spec.Input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.Printf("test %s execution error: %v", spec.Name, err)
	}

	if errors.Is(err, context.DeadlineExceeded) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		result.Error = fmt.Sprintf("timeout after %s", timeout)
		return result
	}
	ro, poErr := parseResultOutput()
	if poErr != nil {
		log.Printf("warning: parse result output for test")
	}
	result.Details = ro

	if err != nil {
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
