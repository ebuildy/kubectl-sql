package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

func sampleEntries() []swaggerEntry {
	return []swaggerEntry{
		{Key: "/v1/pods", Fields: []schema.Field{
			{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
				{Name: "nodeName", Type: schema.FieldTypeString},
			}},
		}},
		{Key: "example.com/v1/widgets", Fields: []schema.Field{
			{Name: "spec", Type: schema.FieldTypeObject},
		}},
	}
}

func TestEncodeGzipGobDeterministic(t *testing.T) {
	entries := sampleEntries()

	first, err := encodeGzipGob(entries)
	require.NoError(t, err)

	second, err := encodeGzipGob(entries)
	require.NoError(t, err)

	assert.Equal(t, first, second, "gzip+gob encoding of the same input must be byte-identical")
}

func TestEncodeGzipGobRoundTrip(t *testing.T) {
	entries := sampleEntries()

	gz, err := encodeGzipGob(entries)
	require.NoError(t, err)

	r, err := gzip.NewReader(bytes.NewReader(gz))
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	var decoded []swaggerEntry
	require.NoError(t, gob.NewDecoder(r).Decode(&decoded))

	assert.Equal(t, entries, decoded)
}

func TestWriteOutputs(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "schema_swagger_k8s_standard_resources.go")
	binPath := filepath.Join(dir, "schema_swagger_k8s_standard_resources.bin.gz")

	require.NoError(t, writeOutputs(outPath, sampleEntries()))

	goSrc, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(goSrc), "//go:embed schema_swagger_k8s_standard_resources.bin.gz")
	assert.Contains(t, string(goSrc), "package k8s")

	first, err := os.ReadFile(binPath)
	require.NoError(t, err)

	// Re-running the generator must reproduce byte-identical outputs.
	require.NoError(t, writeOutputs(outPath, sampleEntries()))
	second, err := os.ReadFile(binPath)
	require.NoError(t, err)

	assert.Equal(t, first, second, "make generate must be reproducible")
}
