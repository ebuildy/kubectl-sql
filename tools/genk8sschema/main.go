// Command genk8sschema converts a pinned Kubernetes OpenAPI v2 ("swagger.json")
// document into the embedded schema snapshot consumed by
// internal/adapter/datasources/k8s/schema_swagger_loader.go.
//
// The fixture lives at internal/adapter/datasources/k8s/testdata/swagger.json
// and is not checked into git; if it is missing, run() downloads it from
// swaggerJSONURL. Its info.version field is "unversioned", but the presence
// of the io.k8s.api.resource.v1 and io.k8s.api.coordination.v1alpha2 groups
// places it around Kubernetes 1.34. Delete that file and re-run `make
// generate` to refresh it (and the embedded snapshot) from the latest
// Kubernetes master.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// swaggerJSONURL is downloaded to -in when that path does not exist.
const swaggerJSONURL = "https://raw.githubusercontent.com/kubernetes/kubernetes/refs/heads/master/api/openapi-spec/swagger.json"

func main() {
	in := flag.String("in", "", "path to the pinned swagger.json (Kubernetes OpenAPI v2 document)")
	out := flag.String("out", "", "path to the generated .go file (a sibling .bin.gz asset is also written)")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: genk8sschema -in swagger.json -out schema_swagger_k8s_standard_resources.go")
		os.Exit(1)
	}

	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "genk8sschema:", err)
		os.Exit(1)
	}
}

func run(inPath, outPath string) error {
	if err := ensureSwaggerFile(inPath); err != nil {
		return err
	}

	data, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inPath, err)
	}

	doc, err := parseSwagger(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", inPath, err)
	}

	resources, err := discoverResources(doc)
	if err != nil {
		return fmt.Errorf("discover resources: %w", err)
	}

	entries := make([]swaggerEntry, 0, len(resources))
	for _, r := range resources {
		entries = append(entries, swaggerEntry{
			Key:    r.key(),
			Fields: defToFields(doc.Definitions, r.defName),
		})
	}

	return writeOutputs(outPath, entries)
}

// ensureSwaggerFile downloads the Kubernetes OpenAPI v2 document to path if
// it does not already exist.
func ensureSwaggerFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	fmt.Fprintf(os.Stderr, "genk8sschema: %s not found, downloading from %s\n", path, swaggerJSONURL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, swaggerJSONURL, nil)
	if err != nil {
		return fmt.Errorf("download %s: %w", swaggerJSONURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", swaggerJSONURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", swaggerJSONURL, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("download %s: %w", swaggerJSONURL, err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}
