package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webPort "github.com/ebuildy/kubectl-sql/internal/port/web"
)

// fakeRunner is a QueryRunner stub returning a canned result/error.
type fakeRunner struct {
	result webPort.QueryResult
	err    error
	called bool
}

func (f *fakeRunner) RunJSON(_ context.Context, _ string) (webPort.QueryResult, error) {
	f.called = true
	return f.result, f.err
}

// fakeCompleter is a Completer stub returning canned candidates.
type fakeCompleter struct {
	candidates []string
}

func (f *fakeCompleter) Complete(_ string, _ int) []string { return f.candidates }

func newTestServer(runner webPort.QueryRunner, completer webPort.Completer) *httptest.Server {
	s := NewServer(runner, completer, "127.0.0.1:0", nil)
	return httptest.NewServer(s.Handler())
}

func postQuery(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/query", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/query: %v", err)
	}
	return resp
}

func TestQuery_Success(t *testing.T) {
	runner := &fakeRunner{result: webPort.QueryResult{
		Columns: []string{"name", "namespace"},
		Rows:    []map[string]any{{"name": "pod-a", "namespace": "default"}},
	}}
	srv := newTestServer(runner, &fakeCompleter{})
	defer srv.Close()

	resp := postQuery(t, srv, `{"sql":"SELECT name, namespace FROM pods"}`)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var got queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Columns) != 2 || got.Columns[0] != "name" {
		t.Fatalf("columns = %v", got.Columns)
	}
	if len(got.Rows) != 1 || got.Rows[0]["name"] != "pod-a" {
		t.Fatalf("rows = %v", got.Rows)
	}
}

func TestQuery_EmptyResult(t *testing.T) {
	runner := &fakeRunner{result: webPort.QueryResult{Columns: []string{"name"}, Rows: nil}}
	srv := newTestServer(runner, &fakeCompleter{})
	defer srv.Close()

	resp := postQuery(t, srv, `{"sql":"SELECT name FROM pods WHERE 1=0"}`)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Rows == nil {
		t.Fatalf("rows should be a non-nil empty array")
	}
	if len(got.Rows) != 0 {
		t.Fatalf("rows = %v, want empty", got.Rows)
	}
}

func TestQuery_ParseError(t *testing.T) {
	runner := &fakeRunner{err: &webPort.Error{Message: "syntax error near FRM"}}
	srv := newTestServer(runner, &fakeCompleter{})
	defer srv.Close()

	resp := postQuery(t, srv, `{"sql":"SELECT name FRM pods"}`)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var got errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error == "" {
		t.Fatalf("error message should be populated")
	}
}

func TestQuery_SuggestionPassthrough(t *testing.T) {
	runner := &fakeRunner{err: &webPort.Error{
		Message:      "field staus does not exist, did you mean status?",
		Suggestion:   "field staus does not exist, did you mean status?",
		CorrectedSQL: "SELECT status FROM pods",
	}}
	srv := newTestServer(runner, &fakeCompleter{})
	defer srv.Close()

	resp := postQuery(t, srv, `{"sql":"SELECT staus FROM pods"}`)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var got errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CorrectedSQL != "SELECT status FROM pods" {
		t.Fatalf("correctedSql = %q", got.CorrectedSQL)
	}
	if got.Suggestion == "" {
		t.Fatalf("suggestion should be populated")
	}
}

func TestQuery_MalformedBody(t *testing.T) {
	runner := &fakeRunner{}
	srv := newTestServer(runner, &fakeCompleter{})
	defer srv.Close()

	for _, body := range []string{`{not json`, `{}`, `{"sql":""}`} {
		resp := postQuery(t, srv, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	if runner.called {
		t.Fatalf("runner must not be called for malformed bodies")
	}
}

func TestQuery_DeleteForbidden(t *testing.T) {
	runner := &fakeRunner{}
	srv := newTestServer(runner, &fakeCompleter{})
	defer srv.Close()

	resp := postQuery(t, srv, `{"sql":"DELETE FROM pods WHERE name = 'x'"}`)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if runner.called {
		t.Fatalf("runner must not be called for DELETE")
	}
}

func TestComplete_Candidates(t *testing.T) {
	srv := newTestServer(&fakeRunner{}, &fakeCompleter{candidates: []string{"pods", "podtemplates"}})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/complete?line=SELECT+name+FROM+po&pos=18")
	if err != nil {
		t.Fatalf("GET /api/complete: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var got completeResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Candidates) != 2 || got.Candidates[0] != "pods" {
		t.Fatalf("candidates = %v", got.Candidates)
	}
}

func TestComplete_Empty(t *testing.T) {
	srv := newTestServer(&fakeRunner{}, &fakeCompleter{candidates: nil})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/complete?line=SELECT&pos=6")
	if err != nil {
		t.Fatalf("GET /api/complete: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var got completeResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Candidates == nil {
		t.Fatalf("candidates should be a non-nil empty array")
	}
	if len(got.Candidates) != 0 {
		t.Fatalf("candidates = %v, want empty", got.Candidates)
	}
}

func TestIndex_ServesEmbeddedPage(t *testing.T) {
	srv := newTestServer(&fakeRunner{}, &fakeCompleter{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(strings.ToLower(string(buf[:n])), "doctype html") {
		t.Fatalf("body does not look like the HTML page: %q", string(buf[:n]))
	}
}
