package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/reyoung/agent-envs/envlet/pkg/executor"
	"github.com/reyoung/agent-envs/envlet/pkg/jobqueue"
	"github.com/reyoung/agent-envs/envlet/pkg/reporter"
)

type Request struct {
	QueueName      string   `json:"queue_name"`
	Binary         []byte   `json:"binary"`
	CapturePattern string   `json:"capture_pattern,omitempty"`
	Args           []string `json:"args,omitempty"`
}

type Response struct {
	Result *executor.ExecuteResult `json:"result,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

const defaultTTL = 30 * time.Minute

func main() {
	var (
		flagListenAddr    = flag.String("listen", ":8080", "HTTP listen address")
		flagRedisDSN      = flag.String("redis", "", "Redis DSN without queue suffix")
		flagCallbackBase  = flag.String("callback-base-url", "", "Base URL for callback endpoint (e.g. http://proxy:8080)")
		flagCallbackTTL   = flag.Duration("callback-ttl", defaultTTL, "TTL for unmatched callbacks")
		flagShutdownGrace = flag.Duration("shutdown-timeout", 15*time.Second, "Graceful shutdown timeout")
	)
	flag.Parse()

	if *flagRedisDSN == "" {
		log.Fatalf("--redis is required")
	}
	if *flagCallbackBase == "" {
		log.Fatalf("--callback-base-url is required")
	}

	callbackBase := strings.TrimRight(*flagCallbackBase, "/")
	if callbackBase == "" {
		log.Fatalf("callback base URL cannot be empty")
	}

	logger := log.New(os.Stdout, "[proxy] ", log.LstdFlags|log.Lmsgprefix)

	publisher, err := newRedisPublisher(*flagRedisDSN)
	if err != nil {
		logger.Fatalf("connect redis: %v", err)
	}
	defer publisher.Close()

	store := newResponseStore(*flagCallbackTTL)
	defer store.Close()

	server := &proxyServer{
		callbackBase: callbackBase,
		publisher:    publisher,
		store:        store,
		logger:       logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/execute", server.handleExecute)
	mux.HandleFunc("/callback/", server.handleCallback)

	httpServer := &http.Server{
		Addr:    *flagListenAddr,
		Handler: mux,
	}

	go func() {
		logger.Printf("listening on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http server: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	logger.Printf("received %s, shutting down", sig)
	ctx, cancel := context.WithTimeout(context.Background(), *flagShutdownGrace)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}
}

type proxyServer struct {
	callbackBase string
	publisher    *redisPublisher
	store        *responseStore
	logger       *log.Logger
}

func (s *proxyServer) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	req.QueueName = strings.TrimSpace(req.QueueName)
	if req.QueueName == "" {
		http.Error(w, "queue_name is required", http.StatusBadRequest)
		return
	}
	if len(req.Binary) == 0 {
		http.Error(w, "binary is required", http.StatusBadRequest)
		return
	}

	jobID := uuid.NewString()
	callbackURL := fmt.Sprintf("%s/callback/%s", s.callbackBase, jobID)

	waitCtx := r.Context()
	waiter, err := s.store.Waiter(jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("prepare waiter: %v", err), http.StatusInternalServerError)
		return
	}
	defer waiter.Close()

	spec := &jobqueue.JobSpec{
		CallbackURL:    callbackURL,
		ID:             jobID,
		Binary:         req.Binary,
		CapturePattern: req.CapturePattern,
		Args:           req.Args,
	}

	if err := s.publisher.Enqueue(waitCtx, req.QueueName, spec); err != nil {
		http.Error(w, fmt.Sprintf("enqueue job: %v", err), http.StatusInternalServerError)
		return
	}

	resp, err := waiter.Wait(waitCtx)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		http.Error(w, fmt.Sprintf("wait for callback: %v", err), status)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *proxyServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/callback/")
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}

	var payload reporter.ReportPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	if payload.ID != "" && payload.ID != jobID {
		http.Error(w, "job id mismatch", http.StatusBadRequest)
		return
	}
	if payload.ID == "" {
		payload.ID = jobID
	}

	resp := Response{
		Result: payload.Exec.Result,
		Error:  payload.Exec.Error,
	}
	s.store.Deliver(payload.ID, resp)

	w.WriteHeader(http.StatusNoContent)
}

type waiter struct {
	id    string
	store *responseStore
	ch    chan Response
}

func (w *waiter) Wait(ctx context.Context) (Response, error) {
	select {
	case resp := <-w.ch:
		return resp, nil
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
}

func (w *waiter) Close() {
	w.store.release(w.id, w.ch)
}

type responseStore struct {
	mu      sync.Mutex
	entries map[string]*storeEntry
	ttl     time.Duration
	stopCh  chan struct{}
}

type storeEntry struct {
	ch     chan Response
	resp   *Response
	expiry time.Time
}

func newResponseStore(ttl time.Duration) *responseStore {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	s := &responseStore{
		entries: make(map[string]*storeEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *responseStore) Close() {
	close(s.stopCh)
}

func (s *responseStore) Waiter(id string) (*waiter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry == nil {
		entry = &storeEntry{}
		s.entries[id] = entry
	}

	if entry.ch != nil {
		return nil, fmt.Errorf("waiter already registered for %s", id)
	}

	if entry.resp != nil {
		// result already available; create closed channel delivering immediately
		ch := make(chan Response, 1)
		ch <- *entry.resp
		delete(s.entries, id)
		return &waiter{id: id, store: s, ch: ch}, nil
	}

	entry.ch = make(chan Response, 1)
	entry.expiry = time.Time{}
	return &waiter{id: id, store: s, ch: entry.ch}, nil
}

func (s *responseStore) Deliver(id string, resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry == nil {
		entry = &storeEntry{}
		s.entries[id] = entry
	}

	if entry.ch != nil {
		select {
		case entry.ch <- resp:
		default:
		}
		entry.ch = nil
		delete(s.entries, id)
		return
	}

	respCopy := resp
	entry.resp = &respCopy
	entry.expiry = time.Now().Add(s.ttl)
}

func (s *responseStore) release(id string, ch chan Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[id]
	if entry == nil {
		return
	}

	if entry.ch == ch {
		entry.ch = nil
		if entry.resp == nil {
			entry.expiry = time.Now().Add(s.ttl)
		}
		if entry.resp == nil {
			// nothing to keep
			delete(s.entries, id)
		}
	}
}

func (s *responseStore) cleanupLoop() {
	interval := s.ttl / 2
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for id, entry := range s.entries {
				if entry.ch != nil {
					continue
				}
				if entry.resp == nil {
					delete(s.entries, id)
					continue
				}
				if !entry.expiry.IsZero() && now.After(entry.expiry) {
					delete(s.entries, id)
				}
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}

type redisPublisher struct {
	client *redis.Client
}

func newRedisPublisher(dsn string) (*redisPublisher, error) {
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
