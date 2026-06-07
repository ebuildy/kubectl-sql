package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// dataSource is the client-go implementation of the k8s.DataSource port.
type dataSource struct {
	dyn      dynamic.Interface
	mapper   meta.RESTMapper
	disco    discovery.DiscoveryInterface
	inferrer schemaInferrer
}

// New builds a client-go-backed DataSource from kubeconfig + context.
// It is the single wiring entry point; the returned value is port-typed.
func New(kubeconfig, kubeContext, namespace string) (k8s.DataSource, error) {
	dyn, mapper, disco, err := newDynamicClient(kubeconfig, kubeContext)
	if err != nil {
		return nil, err
	}
	inferrer := newCompositeInferrer(
		newOpenAPIInferrer(disco),
		newSampleInferrer(dyn, namespace),
	)
	return &dataSource{dyn: dyn, mapper: mapper, disco: disco, inferrer: inferrer}, nil
}

// gvrFor maps a domain Resource back to a GroupVersionResource.
func gvrFor(r k8s.Resource) k8sschema.GroupVersionResource {
	return k8sschema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Name}
}

// Resolve maps a user-typed table name to a canonical Resource.
func (d *dataSource) Resolve(_ context.Context, table string) (k8s.Resource, error) {
	gvr, err := d.mapper.ResourceFor(k8sschema.GroupVersionResource{Resource: strings.ToLower(table)})
	if err != nil {
		return k8s.Resource{}, fmt.Errorf("k8s: resolve resource %q: %w", table, err)
	}
	namespaced := true
	if gvk, kerr := d.mapper.KindFor(gvr); kerr == nil {
		if m, merr := d.mapper.RESTMapping(gvk.GroupKind(), gvk.Version); merr == nil {
			namespaced = m.Scope.Name() == meta.RESTScopeNameNamespace
		}
	}
	return k8s.Resource{
		Name:       gvr.Resource,
		Group:      gvr.Group,
		Version:    gvr.Version,
		Namespaced: namespaced,
	}, nil
}

// Resources enumerates all queryable resources (same filtering as SHOW TABLES).
func (d *dataSource) Resources(_ context.Context) ([]k8s.Resource, error) {
	lists, err := d.disco.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("k8s: list API resources: %w", err)
	}
	var out []k8s.Resource
	for _, list := range lists {
		group, version := splitGroupVersion(list.GroupVersion)
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // skip subresources like pods/log
			}
			out = append(out, k8s.Resource{
				Name:       r.Name,
				Namespaced: r.Namespaced,
				Aliases:    append([]string(nil), r.ShortNames...),
				Group:      group,
				Version:    version,
			})
		}
	}
	return out, nil
}

// InferSchema returns the column model for a resource.
func (d *dataSource) InferSchema(ctx context.Context, r k8s.Resource) ([]schema.Field, error) {
	return d.inferrer.InferFields(ctx, gvrFor(r))
}

// List streams a resource's objects as plain maps, one page per pageFn call.
func (d *dataSource) List(ctx context.Context, r k8s.Resource, opts k8s.ListOptions, pageFn func(page []map[string]any) error) error {
	log := logger.FromContext(ctx)
	ri := d.dyn.Resource(gvrFor(r))

	var continueToken string
	page := 0
	for {
		listOpts := metav1.ListOptions{Limit: opts.PageSize, Continue: continueToken}
		pageStart := time.Now()

		var items []map[string]any
		var nextToken string

		if opts.Namespace != "" {
			list, err := ri.Namespace(opts.Namespace).List(ctx, listOpts)
			if err != nil {
				return fmt.Errorf("k8s: list %s: %w", r.Name, err)
			}
			for i := range list.Items {
				items = append(items, list.Items[i].Object)
			}
			nextToken = list.GetContinue()
		} else {
			list, err := ri.List(ctx, listOpts)
			if err != nil {
				return fmt.Errorf("k8s: list %s: %w", r.Name, err)
			}
			for i := range list.Items {
				items = append(items, list.Items[i].Object)
			}
			nextToken = list.GetContinue()
		}

		log.Debug("listed resource page",
			logger.String("resource", r.Name),
			logger.Int("page", page),
			logger.Int("items", len(items)),
			logger.Duration("elapsed", time.Since(pageStart)))
		page++

		if err := pageFn(items); err != nil {
			return err
		}

		continueToken = nextToken
		if continueToken == "" {
			return nil
		}
	}
}

// splitGroupVersion splits "group/version" or "version" (core) into parts.
func splitGroupVersion(gv string) (group, version string) {
	if idx := strings.LastIndex(gv, "/"); idx >= 0 {
		return gv[:idx], gv[idx+1:]
	}
	return "", gv
}
