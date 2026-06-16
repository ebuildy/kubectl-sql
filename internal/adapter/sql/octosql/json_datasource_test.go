package octosql

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// writeJSONLFile writes one JSON object per line to dir/name and returns the
// full path.
func writeJSONLFile(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := strings.Join(lines, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// executeJSON runs sql against eng and decodes the JSON-formatted result.
func executeJSON(t *testing.T, eng portsql.Engine, sql string) []map[string]any {
	t.Helper()
	var buf strings.Builder
	err := eng.Execute(context.Background(), portsql.Query{SQL: sql}, &buf)
	require.NoError(t, err, "execute %q", sql)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "output is JSON: %s", buf.String())
	return rows
}

// TestJSONFile_SelectStar_DefaultPrefix confirms a .json file reads via FROM
// and its columns are rendered with a <basename>.<field> prefix, the same
// <table>.<field> convention used for Kubernetes-backed tables.
func TestJSONFile_SelectStar_DefaultPrefix(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "notes.json",
		`{"pod":"nginx-1","note":"ok"}`,
		`{"pod":"nginx-2","note":"check logs"}`,
	)

	eng := New(portsql.Config{Output: "json"}, newFakeDataSource(), nil)
	rows := executeJSON(t, eng, "SELECT * FROM "+path)

	require.Len(t, rows, 2)
	assert.Equal(t, "nginx-1", rows[0]["notes.pod"])
	assert.Equal(t, "ok", rows[0]["notes.note"])
	assert.Equal(t, "nginx-2", rows[1]["notes.pod"])
	assert.Equal(t, "check logs", rows[1]["notes.note"])
}

// TestJSONFile_Extensions confirms .json, .jsonl, and .ndjson are all read the
// same way (same schema inference, same row data). An explicit AS alias is
// used so the output columns are named identically (n.pod / n.note) across
// all three extensions.
func TestJSONFile_Extensions(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"pod":"nginx-1","note":"ok"}`,
		`{"pod":"nginx-2","note":"check logs"}`,
	}

	for _, ext := range []string{"json", "jsonl", "ndjson"} {
		t.Run(ext, func(t *testing.T) {
			path := writeJSONLFile(t, dir, "notes."+ext, lines...)

			eng := New(portsql.Config{Output: "json"}, newFakeDataSource(), nil)
			rows := executeJSON(t, eng, "SELECT * FROM "+path+" AS n")

			require.Len(t, rows, 2)
			assert.Equal(t, "nginx-1", rows[0]["n.pod"])
			assert.Equal(t, "ok", rows[0]["n.note"])
			assert.Equal(t, "nginx-2", rows[1]["n.pod"])
			assert.Equal(t, "check logs", rows[1]["n.note"])
		})
	}
}

// TestJSONFile_ColumnSelectionAndWhere checks that an explicit column list and
// a WHERE filter on a JSON field work.
func TestJSONFile_ColumnSelectionAndWhere(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "notes.jsonl",
		`{"pod":"nginx-1","note":"ok"}`,
		`{"pod":"nginx-2","note":"check logs"}`,
	)

	eng := New(portsql.Config{Output: "json"}, newFakeDataSource(), nil)
	rows := executeJSON(t, eng, "SELECT pod, note FROM "+path+" AS n WHERE pod = 'nginx-1'")

	require.Len(t, rows, 1)
	assert.Equal(t, "nginx-1", rows[0]["n.pod"])
	assert.Equal(t, "ok", rows[0]["n.note"])
}

// TestJSONFile_OrderByLimit checks ORDER BY and LIMIT with the -o json output
// format.
func TestJSONFile_OrderByLimit(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "notes.jsonl",
		`{"pod":"nginx-1","note":"ok"}`,
		`{"pod":"nginx-2","note":"check logs"}`,
	)

	eng := New(portsql.Config{Output: "json"}, newFakeDataSource(), nil)
	rows := executeJSON(t, eng, "SELECT * FROM "+path+" AS n ORDER BY pod DESC LIMIT 1")

	require.Len(t, rows, 1)
	assert.Equal(t, "nginx-2", rows[0]["n.pod"])
	assert.Equal(t, "check logs", rows[0]["n.note"])
}

// TestJSONFile_Join checks that a single query can JOIN two local JSON Lines
// files on a shared key.
func TestJSONFile_Join(t *testing.T) {
	dir := t.TempDir()
	notesPath := writeJSONLFile(t, dir, "notes.jsonl",
		`{"pod":"nginx-1","note":"ok"}`,
		`{"pod":"nginx-2","note":"check logs"}`,
	)
	statusPath := writeJSONLFile(t, dir, "status.jsonl",
		`{"pod":"nginx-1","status":"Running"}`,
		`{"pod":"nginx-2","status":"Pending"}`,
	)

	eng := New(portsql.Config{Output: "json"}, newFakeDataSource(), nil)
	rows := executeJSON(t, eng,
		"SELECT n.pod, n.note, s.status FROM "+notesPath+" n JOIN "+statusPath+" s ON n.pod = s.pod")

	require.Len(t, rows, 2)
	byPod := map[string]map[string]any{}
	for _, r := range rows {
		byPod[r["n.pod"].(string)] = r
	}
	assert.Equal(t, "ok", byPod["nginx-1"]["n.note"])
	assert.Equal(t, "Running", byPod["nginx-1"]["s.status"])
	assert.Equal(t, "check logs", byPod["nginx-2"]["n.note"])
	assert.Equal(t, "Pending", byPod["nginx-2"]["s.status"])
}

// TestJSONFile_NonLinesArrayErrors checks that a file which is a single
// top-level JSON array (not JSON Lines) fails with a clear error rather than
// crashing the process.
func TestJSONFile_NonLinesArrayErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(path, []byte("[\n  {\"a\": 1},\n  {\"a\": 2}\n]\n"), 0o644))

	eng := New(portsql.Config{Output: "json"}, newFakeDataSource(), nil)
	var buf strings.Builder
	err := eng.Execute(context.Background(), portsql.Query{SQL: "SELECT * FROM " + path}, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "couldn't parse json")
}
