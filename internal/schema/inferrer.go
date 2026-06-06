// Package schema provides schema inference for Kubernetes resources.
// The SchemaInferrer interface (port.go) is the public contract.
// Concrete adapters: SampleInferrer, OpenAPIInferrer, CompositeInferrer.
package schema

// InferFields derives a field list from a sample unstructured Kubernetes object.
// Guaranteed fields (name, namespace, raw) are always prepended.
// Returns nil when obj is nil or empty — callers should fall back to a static schema.
// This is a convenience wrapper used in tests and by SampleInferrer internally.
func InferFields(obj map[string]interface{}) []Field {
	walked := walkObject(obj)
	if walked == nil {
		return nil
	}
	fields := make([]Field, len(guaranteedFields))
	copy(fields, guaranteedFields)
	return append(fields, walked...)
}
