// Package web is the primary/driving HTTP adapter for the web UI. It serves a
// single embedded HTML page plus a small JSON API, driving the QueryRunner and
// Completer ports (internal/port/web). It owns no cluster/SQL logic: those are
// injected as ports, so the handlers are testable with fakes and import no
// k8s/octosql code.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"

	webPort "github.com/ebuildy/kubectl-sql/internal/port/web"
)

// assetsFS holds the minified, embedded front-end. The editable sources live
// under assets/; `make web-assets` minifies them into dist/, which is what ships
// in the binary. dist/ is generated and git-ignored (never committed) — only a
// .gitkeep is tracked so this `all:dist` embed always compiles on a fresh
// checkout; build/test/CI regenerate the real files first. Edit assets/, then
// run `make web-assets`.
//
//go:embed all:dist
var assetsFS embed.FS

// Server is the HTTP adapter. Build it with NewServer and drive its lifecycle
// with Listen/Serve (or Start) and Shutdown.
type Server struct {
	runner     webPort.QueryRunner
	completer  webPort.Completer
	addr       string
	isMutating func(sql string) bool
	httpServer *http.Server
}

// NewServer builds the HTTP server from the query and completion ports and a
// bind address. mutationGuard classifies a statement as mutating (rejected at
// the API boundary); when nil it defaults to a DELETE check, keeping the
// browser surface read-only.
func NewServer(runner webPort.QueryRunner, completer webPort.Completer, addr string, mutationGuard func(sql string) bool) *Server {
	if mutationGuard == nil {
		mutationGuard = defaultMutationGuard
	}
	s := &Server{runner: runner, completer: completer, addr: addr, isMutating: mutationGuard}
	s.httpServer = &http.Server{Addr: addr, Handler: s.Handler()}
	return s
}

// defaultMutationGuard rejects DELETE statements (first token "delete").
func defaultMutationGuard(sql string) bool {
	fields := strings.Fields(strings.TrimSpace(sql))
	return len(fields) > 0 && strings.EqualFold(fields[0], "delete")
}

// Handler builds the route mux. Exposed so tests can exercise handlers via
// httptest without binding a socket.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)

	sub, _ := fs.Sub(assetsFS, "dist")
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/app.css", fileServer)
	mux.Handle("/app.js", fileServer)

	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/complete", s.handleComplete)
	return mux
}

// Listen binds the configured address, returning the listener (or a bind
// error) so the caller can report the resolved address before serving.
func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", s.addr)
}

// Serve serves HTTP on the given listener until Shutdown. It returns
// http.ErrServerClosed on a clean shutdown.
func (s *Server) Serve(ln net.Listener) error {
	return s.httpServer.Serve(ln)
}

// Start binds and serves in one call (convenience). It returns
// http.ErrServerClosed on a clean shutdown.
func (s *Server) Start() error {
	ln, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := assetsFS.ReadFile("dist/index.html")
	if err != nil {
		http.Error(w, "index unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

type queryRequest struct {
	SQL string `json:"sql"`
}

type queryResponse struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

type errorResponse struct {
	Error        string `json:"error"`
	Suggestion   string `json:"suggestion,omitempty"`
	CorrectedSQL string `json:"correctedSql,omitempty"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	var req queryRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.SQL) == "" {
		writeError(w, http.StatusBadRequest, errorResponse{Error: "invalid request body: expected non-empty {\"sql\": \"...\"}"})
		return
	}

	// Mutation guard: reject DELETE/mutating statements before any execution so
	// the browser cannot trigger destructive operations.
	if s.isMutating(req.SQL) {
		writeError(w, http.StatusForbidden, errorResponse{Error: "mutating statements (e.g. DELETE) are not allowed through the web UI; use the CLI's confirmation flow"})
		return
	}

	result, err := s.runner.RunJSON(r.Context(), req.SQL)
	if err != nil {
		resp := errorResponse{Error: err.Error()}
		var we *webPort.Error
		if errors.As(err, &we) {
			resp.Error = we.Message
			resp.Suggestion = we.Suggestion
			resp.CorrectedSQL = we.CorrectedSQL
		}
		writeError(w, http.StatusBadRequest, resp)
		return
	}

	rows := result.Rows
	if rows == nil {
		rows = []map[string]any{}
	}
	cols := result.Columns
	if cols == nil {
		cols = []string{}
	}
	writeJSON(w, http.StatusOK, queryResponse{Columns: cols, Rows: rows})
}

type completeResponse struct {
	Candidates []string `json:"candidates"`
}

func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	line := r.URL.Query().Get("line")
	pos, err := strconv.Atoi(r.URL.Query().Get("pos"))
	if err != nil {
		pos = len([]rune(line))
	}
	candidates := s.completer.Complete(line, pos)
	if candidates == nil {
		candidates = []string{}
	}
	writeJSON(w, http.StatusOK, completeResponse{Candidates: candidates})
}

// writeJSON writes v as a JSON body with the given status and the JSON content
// type, so every /api/* response (success or error) is uniform.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, resp errorResponse) {
	writeJSON(w, status, resp)
}
