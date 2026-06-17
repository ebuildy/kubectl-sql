package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDomainImportsNoAdapter enforces the hexagonal dependency rule: no package
// under internal/domain/ may import any package under internal/adapter/. The
// domain depends only on ports; all concrete adapter wiring lives in this
// package (internal/app). A violation means an adapter leaked back into a use
// case, breaking the composition-root guarantee.
func TestDomainImportsNoAdapter(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	domainRoot := filepath.Join(root, "internal", "domain")
	const adapterPrefix = "github.com/ebuildy/kubectl-sql/internal/adapter/"

	fset := token.NewFileSet()
	var violations []string

	err = filepath.WalkDir(domainRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Tests may legitimately import adapters; the rule is about production code.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, adapterPrefix) {
				rel := strings.TrimPrefix(path, root+string(filepath.Separator))
				violations = append(violations, rel+" -> "+p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("internal/domain must not import internal/adapter:\n  %s",
			strings.Join(violations, "\n  "))
	}
}
