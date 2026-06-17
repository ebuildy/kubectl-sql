package octosql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// TestNewFactory_ProducesWorkingEngine proves the EngineFactory adapter returns
// an Engine that executes a real query against the injected data source and
// renders in the configured output mode.
func TestNewFactory_ProducesWorkingEngine(t *testing.T) {
	eng := NewFactory(mapFakeDS{}, nil).New(portsql.Config{Output: "json"})

	var buf strings.Builder
	err := eng.Execute(context.Background(),
		portsql.Query{SQL: "SELECT metadata->name AS name FROM pods"},
		&buf)
	require.NoError(t, err, "execute: %s", buf.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &rows), "JSON: %s", buf.String())
	require.Len(t, rows, 2)
	assert.Equal(t, "nginx", rows[0]["name"])
	assert.Equal(t, "redis", rows[1]["name"])
}
