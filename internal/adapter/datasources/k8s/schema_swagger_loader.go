package k8s

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"fmt"
	"sync"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// swaggerEntry mirrors tools/genk8sschema's encoding of one resource's field
// tree. gob decoding matches by field name/type, not by package or type
// identity, so this need not be the same Go type as the generator's.
type swaggerEntry struct {
	Key    string
	Fields []schema.Field
}

// swaggerSchemaProvider serves []schema.Field for standard Kubernetes resources
// from an embedded, build-time-generated snapshot of the Kubernetes OpenAPI
// ("swagger.json") spec. See schema_swagger_k8s_standard_resources.go.
type swaggerSchemaProvider struct {
}

func newSwaggerSchemaProvider(ctx context.Context) *swaggerSchemaProvider {
	return &swaggerSchemaProvider{}
}

var (
	swaggerIndexOnce sync.Once
	swaggerIndex     map[string][]schema.Field
	swaggerIndexErr  error
)

// loadSwaggerIndex decompresses and decodes the embedded snapshot at most
// once, caching the result (or error) for subsequent calls.
func loadSwaggerIndex() (map[string][]schema.Field, error) {
	swaggerIndexOnce.Do(func() {
		gz, err := gzip.NewReader(bytes.NewReader(swaggerSchemaDataGzip))
		if err != nil {
			swaggerIndexErr = fmt.Errorf("swagger schema: open gzip: %w", err)
			return
		}
		defer func() { _ = gz.Close() }()

		var entries []swaggerEntry
		if err := gob.NewDecoder(gz).Decode(&entries); err != nil {
			swaggerIndexErr = fmt.Errorf("swagger schema: gob decode: %w", err)
			return
		}

		idx := make(map[string][]schema.Field, len(entries))
		for _, e := range entries {
			idx[e.Key] = e.Fields
		}
		swaggerIndex = idx
	})
	return swaggerIndex, swaggerIndexErr
}

// Provide returns the embedded field tree for gvr, prefixed with the
// guaranteed fields (name, namespace). It returns (nil, nil) — not an error —
// for resources not covered by the embedded snapshot (e.g. CRDs), so callers
// fall through to the live OpenAPI/sample layers unchanged.
func (p *swaggerSchemaProvider) Provide(ctx context.Context, gvr k8sschema.GroupVersionResource) ([]schema.Field, error) {
	log := logger.FromContext(ctx)
	idx, err := loadSwaggerIndex()
	if err != nil {
		log.Error("cannot load openapi data", logger.Err(err))
	} else {
		keys := make([]string, 0, len(idx))
		for k := range idx {
			keys = append(keys, k)
		}
		log.Debug("loaded openapi schemas", logger.String("schemas", fmt.Sprint(keys)))
	}

	fields, ok := idx[swaggerKey(gvr)]
	if !ok {
		return nil, nil
	}

	log.Debug("openapi: found schema")

	return append(schema.GuaranteedFields(), fields...), nil
}

// swaggerKey is the embedded snapshot's lookup key: "<group>/<version>/<resource>",
// with an empty group for the core API group, e.g. "/v1/pods".
func swaggerKey(gvr k8sschema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}
