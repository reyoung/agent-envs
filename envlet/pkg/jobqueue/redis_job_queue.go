package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisJobQueue struct {
	client    *redis.Client
	queueName string
}

func newRedisJobQueue(redisDSN string) (JobQueue, error) {
	target, queueName, err := parseQueueName(redisDSN)
	if err != nil {
		return nil, err
	}

	opts, err := buildRedisOptions(target)
	if err != nil {
		return nil, fmt.Errorf("parse redis options: %w", err)
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	log.Printf("connected to redis, using queue %q", queueName)

	return &redisJobQueue{
		client:    client,
		queueName: queueName,
	}, nil
}

func (q *redisJobQueue) Fetch(ctx context.Context) (*JobSpec, error) {
	begin := time.Now()
	log.Printf("waiting for job from redis queue %q...", q.queueName)
	result, err := q.client.BLPop(ctx, 0, q.queueName).Result()
	log.Printf("waited %v for job from redis queue %q. err %v", time.Since(begin), q.queueName, err)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("fetch from redis queue %q: %w", q.queueName, err)
	}

	if len(result) != 2 {
		return nil, fmt.Errorf("unexpected response from redis queue %q", q.queueName)
	}

	var job JobSpec
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("decode job payload: %w", err)
	}

	return &job, nil
}

func (q *redisJobQueue) Close() error {
	return q.client.Close()
}

func parseQueueName(raw string) (string, string, error) {
	parts := strings.SplitN(raw, "#", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("redis DSN must include '#<queue>' suffix")
	}

	queue := strings.TrimSpace(parts[1])
	if queue == "" {
		return "", "", fmt.Errorf("queue name is required in redis DSN suffix")
	}

	return strings.TrimSpace(parts[0]), queue, nil
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
