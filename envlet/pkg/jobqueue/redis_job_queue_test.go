package jobqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
)

func TestRedisJobQueueFetch(t *testing.T) {
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer srv.Close()

	queueName := "jobs"
	job := JobSpec{CallbackURL: "https://example.com", ID: "1", Binary: []byte("payload")}

	dsns := []string{
		fmt.Sprintf("redis://%s#%s", srv.Addr(), queueName),
		fmt.Sprintf("%s#%s", srv.Addr(), queueName),
	}

	for _, dsn := range dsns {
		t.Run(dsn, func(t *testing.T) {
			srv.FlushAll()

			q, err := New(dsn)
			if err != nil {
				t.Fatalf("create queue: %v", err)
			}
			defer q.Close()

			payload, err := json.Marshal(job)
			if err != nil {
				t.Fatalf("marshal job: %v", err)
			}

			srv.RPush(queueName, string(payload))

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			got, err := q.Fetch(ctx)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}

			if got.CallbackURL != job.CallbackURL || got.ID != job.ID || string(got.Binary) != string(job.Binary) {
				t.Fatalf("unexpected job: %+v", got)
			}
		})
	}
}

func TestParseQueueName(t *testing.T) {
	host := "redis://localhost:6379"
	queue := "jobs"
	addr, qname, err := parseQueueName(fmt.Sprintf("%s#%s", host, queue))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != host {
		t.Fatalf("addr mismatch: %s", addr)
	}
	if qname != queue {
		t.Fatalf("queue mismatch: %s", qname)
	}

	cases := []string{
		"redis://localhost:6379",
		"redis://localhost:6379#",
		"redis://localhost:6379#   ",
	}
	for _, c := range cases {
		if _, _, err := parseQueueName(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestNewFactoryUnsupportedScheme(t *testing.T) {
	if _, err := New("sqs://example#jobs"); err == nil {
		t.Fatalf("expected error for unsupported scheme")
	}
}
