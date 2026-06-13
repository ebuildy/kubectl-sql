package main

import (
	"encoding/json"
	"sort"
	"strings"

	"k8s.io/kube-openapi/pkg/validation/spec"
)

// gvk is the (group, version, kind) tuple carried by the
// "x-kubernetes-group-version-kind" extension, on both path operations
// (a single object) and definitions (an array of objects).
type gvk struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// resource is a single (group, version, resource) triple resolved to the
// swagger definition describing its shape.
type resource struct {
	group, version, name string
	defName              string
}

// key is the lookup key used in the embedded snapshot: "<group>/<version>/<resource>",
// with an empty group for the core API group, e.g. "/v1/pods".
func (r resource) key() string {
	return r.group + "/" + r.version + "/" + r.name
}

func parseSwagger(data []byte) (*spec.Swagger, error) {
	var doc spec.Swagger
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// discoverResources finds every (group, version, resource) covered by a "list"
// path operation and resolves it to the swagger definition for its item kind.
// Resources without a list path, without an "x-kubernetes-group-version-kind"
// extension, or whose kind has no matching definition are skipped.
func discoverResources(doc *spec.Swagger) ([]resource, error) {
	if doc.Paths == nil {
		return nil, nil
	}

	gvkToDef := indexDefinitionsByGVK(doc.Definitions)

	paths := make([]string, 0, len(doc.Paths.Paths))
	for p := range doc.Paths.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	seen := make(map[string]bool)
	var resources []resource
	for _, p := range paths {
		op := doc.Paths.Paths[p].Get
		if op == nil {
			continue
		}

		if action, _ := op.Extensions.GetString("x-kubernetes-action"); action != "list" {
			continue
		}

		var g gvk
		if err := op.Extensions.GetObject("x-kubernetes-group-version-kind", &g); err != nil {
			continue
		}

		defName, ok := gvkToDef[g]
		if !ok {
			continue
		}

		name := lastPathSegment(p)
		if name == "" {
			continue
		}

		r := resource{group: g.Group, version: g.Version, name: name, defName: defName}
		if seen[r.key()] {
			continue
		}
		seen[r.key()] = true
		resources = append(resources, r)
	}

	sort.Slice(resources, func(i, j int) bool { return resources[i].key() < resources[j].key() })
	return resources, nil
}

// indexDefinitionsByGVK maps each (group, version, kind) declared by a
// definition's "x-kubernetes-group-version-kind" extension to that
// definition's name.
func indexDefinitionsByGVK(defs spec.Definitions) map[gvk]string {
	idx := make(map[gvk]string, len(defs))
	for name, def := range defs {
		var gvks []gvk
		if err := def.Extensions.GetObject("x-kubernetes-group-version-kind", &gvks); err != nil {
			continue
		}
		for _, g := range gvks {
			idx[g] = name
		}
	}
	return idx
}

// lastPathSegment returns the last non-parameter segment of a swagger path,
// e.g. "/apis/apps/v1/namespaces/{namespace}/deployments" -> "deployments".
func lastPathSegment(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if seg := parts[i]; seg != "" && !strings.HasPrefix(seg, "{") {
			return seg
		}
	}
	return ""
}
