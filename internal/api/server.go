// Package api provides a REST API for dynamic PID registration.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"write-tracer/internal/pidmgr"
)

// Server provides REST endpoints for managing tracked PIDs.
type Server struct {
	registry *pidmgr.PIDRegistry
	addr     string
}

// RegisterRequest is the JSON payload for registering a PID.
type RegisterRequest struct {
	PID uint32 `json:"pid"`
}

// RegisterResponse is returned after successfully registering a PID.
type RegisterResponse struct {
	PID     uint32 `json:"pid"`
	Threads int    `json:"threads"`
	Message string `json:"message"`
}

// ListResponse is returned by GET /pids.
type ListResponse struct {
	Processes []ProcessInfo `json:"processes"`
	Total     int           `json:"total"`
}

// ProcessInfo contains information about a tracked process.
type ProcessInfo struct {
	PID          uint32 `json:"pid"`
	ThreadCount  int    `json:"thread_count"`
	RegisteredAt string `json:"registered_at"`
}

// ErrorResponse is returned on errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// New creates a new API server bound to the given port.
// It binds to localhost only for security.
func New(registry *pidmgr.PIDRegistry, port int) *Server {
	return &Server{
		registry: registry,
		addr:     fmt.Sprintf("127.0.0.1:%d", port),
	}
}

// Start begins serving the REST API in a goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/pids", s.handlePids)
	mux.HandleFunc("/pids/", s.handlePidByID)

	go func() {
		slog.Info("REST API server starting", "addr", s.addr)
		if err := http.ListenAndServe(s.addr, mux); err != nil {
			slog.Error("REST API server failed", "error", err)
		}
	}()

	return nil
}

// Addr returns the server's listen address.
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) handlePids(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPids(w, r)
	case http.MethodPost:
		s.registerPid(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handlePidByID(w http.ResponseWriter, r *http.Request) {
	// Extract PID from URL path: /pids/12345
	path := strings.TrimPrefix(r.URL.Path, "/pids/")
	if path == "" {
		s.writeError(w, http.StatusBadRequest, "PID required in URL path")
		return
	}

	pid, err := strconv.ParseUint(path, 10, 32)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid PID format")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		s.unregisterPid(w, uint32(pid))
	case http.MethodGet:
		s.getPid(w, uint32(pid))
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listPids(w http.ResponseWriter, _ *http.Request) {
	procs := s.registry.List()

	response := ListResponse{
		Processes: make([]ProcessInfo, len(procs)),
		Total:     len(procs),
	}

	for i, p := range procs {
		response.Processes[i] = ProcessInfo{
			PID:          p.ParentPID,
			ThreadCount:  len(p.ThreadIDs),
			RegisteredAt: p.RegisteredAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) registerPid(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.PID == 0 {
		s.writeError(w, http.StatusBadRequest, "PID is required and must be non-zero")
		return
	}

	threads, err := s.registry.RegisterPID(req.PID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, RegisterResponse{
		PID:     req.PID,
		Threads: threads,
		Message: fmt.Sprintf("Successfully registered PID %d with %d threads", req.PID, threads),
	})
}

func (s *Server) unregisterPid(w http.ResponseWriter, pid uint32) {
	if err := s.registry.UnregisterPID(pid); err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Successfully unregistered PID %d", pid),
	})
}

func (s *Server) getPid(w http.ResponseWriter, pid uint32) {
	procs := s.registry.List()

	for _, p := range procs {
		if p.ParentPID == pid {
			s.writeJSON(w, http.StatusOK, ProcessInfo{
				PID:          p.ParentPID,
				ThreadCount:  len(p.ThreadIDs),
				RegisteredAt: p.RegisteredAt.Format("2006-01-02T15:04:05Z07:00"),
			})
			return
		}
	}

	s.writeError(w, http.StatusNotFound, fmt.Sprintf("PID %d is not registered", pid))
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, ErrorResponse{Error: message})
}
