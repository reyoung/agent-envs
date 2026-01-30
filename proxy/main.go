package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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

type RequestWithID struct {
	ID string `json:"id"`
	Request
}

type ResponseWithID struct {
	ID string `json:"id"`
	Response
}

const defaultTTL = 30 * time.Minute
const (
	batchInitialBuffer = 256 * 1024       // 256KB initial buffer
	batchMaxBuffer     = 64 * 1024 * 1024 // 64MB hard cap
)

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
	mux.HandleFunc("/unordered_batch_execute", server.handleUnorderedBatchExecute)
	mux.HandleFunc("/batch_execute", server.handleBatchExecute)
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
	if err := req.normalize(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.executeRequest(r.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

type waiterOrError struct {
	waiter *waiter
	err    error
}

func (w *waiterOrError) Close() error {
	if w.waiter != nil {
		w.waiter.Close()
	}
	return nil
}

func (s *proxyServer) handleBatchExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reader := bufio.NewScanner(r.Body)
	reader.Buffer(make([]byte, batchInitialBuffer), batchMaxBuffer)

	var waiterOrErrs []waiterOrError
	defer func() {
		for _, we := range waiterOrErrs {
			we.Close()
		}
	}()

	for reader.Scan() {
		select {
		case <-r.Context().Done():
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		default:
		}

		line := bytes.TrimSpace(reader.Bytes())
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			waiterOrErrs = append(waiterOrErrs, waiterOrError{waiter: nil, err: fmt.Errorf("invalid request line: %v", err)})
			continue
		}

		if err := req.normalize(); err != nil {
			waiterOrErrs = append(waiterOrErrs, waiterOrError{waiter: nil, err: err})
			continue
		}

		wtr, _, err := s.prepareJob(r.Context(), req)
		if err != nil {
			waiterOrErrs = append(waiterOrErrs, waiterOrError{waiter: nil, err: fmt.Errorf("prepare job: %v", err)})
			continue
		}
		waiterOrErrs = append(waiterOrErrs, waiterOrError{waiter: wtr, err: nil})
	}

	if err := reader.Err(); err != nil {
		http.Error(w, fmt.Sprintf("read request: %v", err), http.StatusBadRequest)
		return
	}

	if len(waiterOrErrs) == 0 {
		http.Error(w, "empty request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	log.Printf("processing batch of %d requests", len(waiterOrErrs))
	encoder := json.NewEncoder(w)

	for _, wtr := range waiterOrErrs {
		if wtr.err != nil {
			resp := Response{Error: wtr.err.Error()}
			if err := encoder.Encode(resp); err != nil {
				s.logger.Printf("write batch response: %v", err)
				return
			}
			continue
		}

		if wtr.waiter == nil {
			resp := Response{Error: "internal error: missing waiter"}
			if err := encoder.Encode(resp); err != nil {
				s.logger.Printf("write batch response: %v", err)
				return
			}
			continue
		}
		resp, err := wtr.waiter.Wait(r.Context())
		if err != nil {
			resp := Response{Error: fmt.Sprintf("wait for callback: %v", err)}
			if err := encoder.Encode(resp); err != nil {
				s.logger.Printf("write batch response: %v", err)
				return
			}
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			s.logger.Printf("write batch response: %v", err)
			return
		}
	}
}

type jsonEncoder struct {
	mu      sync.Mutex
	encoder *json.Encoder
}

func newJSONEncoder(w io.Writer) *jsonEncoder {
	return &jsonEncoder{
		encoder: json.NewEncoder(w),
	}
}

func (je *jsonEncoder) Encode(v interface{}) error {
	je.mu.Lock()
	defer je.mu.Unlock()
	return je.encoder.Encode(v)
}

func (s *proxyServer) handleUnorderedBatchExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reader := bufio.NewScanner(r.Body)
	reader.Buffer(make([]byte, batchInitialBuffer), batchMaxBuffer)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	encoder := newJSONEncoder(w)
	var complete sync.WaitGroup
	defer func() {
		complete.Wait()
	}()

	for reader.Scan() {
		select {
		case <-r.Context().Done():
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		default:
		}

		line := bytes.TrimSpace(reader.Bytes())
		if len(line) == 0 {
			continue
		}

		var req RequestWithID
		if err := json.Unmarshal(line, &req); err != nil {
			encoder.Encode(&ResponseWithID{ID: "", Response: Response{Error: fmt.Sprintf("invalid request line: %v", err)}})
			return
		}

		req.ID = strings.TrimSpace(req.ID)
		if req.ID == "" {
			encoder.Encode(&ResponseWithID{ID: "", Response: Response{Error: "id is required"}})
			return
		}

		if err := req.Request.normalize(); err != nil {
			encoder.Encode(&ResponseWithID{ID: req.ID, Response: Response{Error: err.Error()}})
			return
		}

		wtr, _, err := s.prepareJob(r.Context(), req.Request)
		if err != nil {
			encoder.Encode(&ResponseWithID{ID: req.ID, Response: Response{Error: fmt.Sprintf("prepare job: %v", err)}})
			return
		}
		complete.Add(1)
		go func() {
			defer complete.Done()
			defer wtr.Close()
			resp, err := wtr.Wait(r.Context())
			if err != nil {
				encoder.Encode(&ResponseWithID{
					ID: req.ID,
					Response: Response{
						Error: fmt.Sprintf("wait for callback: %v", err),
					},
				})
				return
			}
			encoder.Encode(&ResponseWithID{
				ID:       req.ID,
				Response: resp,
			})
		}()
	}

	if err := reader.Err(); err != nil {
		log.Printf("read request: %v", err)
		return
	}
}

func (req *Request) normalize() error {

	req.QueueName = strings.TrimSpace(req.QueueName)
	if req.QueueName == "" {
		return fmt.Errorf("queue_name is required")
	}
	if len(req.Binary) == 0 {
		return fmt.Errorf("binary is required")
	}
	return nil
}

func (s *proxyServer) executeRequest(ctx context.Context, req Request) (Response, error) {
	waiter, _, err := s.prepareJob(ctx, req)
	if err != nil {
		return Response{}, err
	}
	defer waiter.Close()

	resp, err := waiter.Wait(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("wait for callback: %w", err)
	}

	return resp, nil
}

func (s *proxyServer) prepareJob(ctx context.Context, req Request) (*waiter, string, error) {
	jobID := uuid.NewString()
	callbackURL := fmt.Sprintf("%s/callback/%s", s.callbackBase, jobID)

	waiter, err := s.store.Waiter(jobID)
	if err != nil {
		return nil, "", fmt.Errorf("prepare waiter: %w", err)
	}

	spec := &jobqueue.JobSpec{
		CallbackURL:    callbackURL,
		ID:             jobID,
		Binary:         req.Binary,
		CapturePattern: req.CapturePattern,
		Args:           req.Args,
	}

	if err := s.publisher.Enqueue(ctx, req.QueueName, spec); err != nil {
		waiter.Close()
		return nil, "", fmt.Errorf("enqueue job: %w", err)
	}

	return waiter, jobID, nil
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
