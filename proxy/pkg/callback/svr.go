package callback

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/reyoung/agent-envs/envlet/pkg/reporter"
	"github.com/reyoung/agent-envs/proxy/pkg/model"
	"github.com/reyoung/agent-envs/proxy/pkg/response_store"
)

type Server struct {
	Store  response_store.ResponseStore
	server *http.Server
}

func (s *Server) Start(address string) {
	if s.server != nil {
		panic("server already started")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback/", s.handleCallback)
	s.server = &http.Server{
		Addr:    address,
		Handler: mux,
	}
	go s.server.ListenAndServe()
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
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

	resp := model.Response{
		Result: payload.Exec.Result,
		Error:  payload.Exec.Error,
	}
	s.Store.Deliver(payload.ID, resp)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) Close() error {
	return s.server.Close()
}
