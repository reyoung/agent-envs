package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/reyoung/agent-envs/envlet/pkg/executor"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
	"github.com/reyoung/agent-envs/envlet/pkg/reporter"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <redis_dsn>\n", os.Args[0])
		os.Exit(2)
	}
	redisDSN := os.Args[1]

	queue, err := jobqueue.New(redisDSN)
	if err != nil {
		log.Fatalf("create job queue: %v", err)
	}
	defer queue.Close()

	exec, err := executor.New()
	if err != nil {
		log.Fatalf("create executor: %v", err)
	}
	defer exec.Close()

	report := reporter.NewHTTPCallbackReporter(nil)
	defer report.Close()

	fetchCtx, cancelFetch := context.WithCancel(context.Background())
	defer cancelFetch()

	var stop atomic.Bool
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		sig := <-sigCh
		log.Printf("received %s, shutting down", sig)
		stop.Store(true)
		cancelFetch()
	}()

	execCtx := context.Background()
	for {
		if stop.Load() {
			break
		}

		job, err := queue.Fetch(fetchCtx)
		if err != nil {
			log.Printf("fetch job: %v", err)
			if fetchCtx.Err() != nil && stop.Load() {
				break
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				if stop.Load() {
					break
				}
				continue
			}
			continue
		}

		if job == nil {
			continue
		}

		log.Printf("fetched job %s", job.ID)

		result, execErr := exec.Execute(execCtx, job)
		if execErr != nil && result == nil {
			log.Printf("execute job %s: %v", job.ID, execErr)
		}

		if err := report.Report(execCtx, job, result, execErr); err != nil {
			log.Printf("report job %s: %v", job.ID, err)
		}

		if stop.Load() {
			break
		}
	}
}
