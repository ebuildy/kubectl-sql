package k8s

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// k8sLibPrefixes are the Kubernetes client libraries that must not leak past the
// adapter boundary.
var k8sLibPrefixes = []string{
	"k8s.io/client-go",
	"k8s.io/apimachinery",
	"k8s.io/kube-openapi",
}

// TestK8sImportBoundary enforces the hexagonal boundary: Kubernetes client
// libraries may be imported only by this adapter package and the cmd composition
// root that wires it. Any other importer means client-go has leaked across the
// port and the spec's isolation guarantee is broken.
func TestK8sImportBoundary(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	allowedPrefixes := []string{
		filepath.Join(root, "internal", "adapter", "datasources", "k8s"),
		filepath.Join(root, "cmd"),
		// Dev-time code generators are separate `main` packages outside the
		// runtime binary's hexagonal boundary (e.g. tools/genk8sschema parses
		// swagger.json at build time to produce the embedded schema snapshot).
		filepath.Join(root, "tools"),
	}

	fset := token.NewFileSet()
	var violations []string

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "external", "bin", ".git", "vendor", "test":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if isK8sLib(p) && !allowed(path, allowedPrefixes) {
				violations = append(violations, strings.TrimPrefix(path, root+string(filepath.Separator)))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("Kubernetes client libraries imported outside the adapter/cmd boundary:\n  %s",
			strings.Join(uniq(violations), "\n  "))
	}
}

func isK8sLib(pkg string) bool {
	for _, pre := range k8sLibPrefixes {
		if pkg == pre || strings.HasPrefix(pkg, pre+"/") {
			return true
		}
	}
	return false
}

func allowed(path string, prefixes []string) bool {
	for _, pre := range prefixes {
		if path == pre || strings.HasPrefix(path, pre+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
