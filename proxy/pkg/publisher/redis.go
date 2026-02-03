package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
)

type redisPublisher struct {
	client *redis.Client
}

func New(dsn string) (Publisher, error) {
	opts, err := buildRedisOptions(dsn)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &redisPublisher{client: client}, nil
}

func (p *redisPublisher) Close() error {
	return p.client.Close()
}

func (p *redisPublisher) Enqueue(ctx context.Context, queue string, spec *jobqueue.JobSpec) error {
	if spec == nil {
		return fmt.Errorf("job spec is nil")
	}
	queue = strings.TrimSpace(queue)
	if queue == "" {
		return fmt.Errorf("queue name is empty")
	}

	payload, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode job spec: %w", err)
	}

	if err := p.client.RPush(ctx, queue, payload).Err(); err != nil {
		return fmt.Errorf("push job to queue %q: %w", queue, err)
	}
	return nil
}

func buildRedisOptions(dsn string) (*redis.Options, error) {
	if strings.HasPrefix(dsn, "redis://") || strings.HasPrefix(dsn, "rediss://") {
		return redis.ParseURL(dsn)
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		dsn = "127.0.0.1:6379"
	}
	return &redis.Options{Addr: dsn}, nil
}
