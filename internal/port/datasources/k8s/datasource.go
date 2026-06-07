// Package k8s defines the Kubernetes data-source port: a library-free interface
// for resolving, listing, and inferring the schema of Kubernetes resources.
//
// No exported type or method here references k8s.io/* — resources, rows, and
// schema are expressed in plain Go and domain types. The client-go binding lives
// behind the adapter in internal/adapter/datasources/k8s, so the data source can
// be swapped without touching consumers.
package k8s

import (
	"context"

	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// Resource is a canonical, library-free identity for a queryable kind.
type Resource struct {
	Name       string   // canonical plural, e.g. "pods"
	Namespaced bool     // whether the resource is namespaced
	Aliases    []string // short names, e.g. "po"
	Group      string   // API group ("" for core)
	Version    string   // API version, e.g. "v1"
}

// ListOptions controls a List call.
type ListOptions struct {
	Namespace string
	PageSize  int64
}

// DataSource is the Kubernetes data-source port. Consumers depend only on this
// interface; the concrete client-go implementation lives in the adapter package.
type DataSource interface {
	// Resolve maps a user-typed table name (plural/short/kind) to a Resource.
	Resolve(ctx context.Context, table string) (Resource, error)
	// Resources enumerates all queryable resources (for SHOW TABLES / completion).
	Resources(ctx context.Context) ([]Resource, error)
	// InferSchema returns the column model for a resource.
	InferSchema(ctx context.Context, r Resource) ([]schema.Field, error)
	// List streams a resource's objects as plain maps, honoring namespace and page
	// size. pageFn is called once per page so callers can stream without buffering
	// the whole cluster in memory.
	List(ctx context.Context, r Resource, opts ListOptions, pageFn func(page []map[string]any) error) error
}
