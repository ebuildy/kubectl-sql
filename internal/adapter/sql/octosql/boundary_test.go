package octosql

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOctosqlImportBoundary enforces the hexagonal boundary: github.com/cube2222/octosql
// may be imported only by this adapter package. Any other importer means the SQL
// engine library has leaked across the port and the spec's isolation guarantee is
// broken. The test/ tree is excluded — e2e tests assert on rendered output, not
// octosql types.
func TestOctosqlImportBoundary(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	allowedPrefix := filepath.Join(root, "internal", "adapter", "sql", "octosql")

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
			if strings.HasPrefix(p, "github.com/cube2222/octosql") {
				if path != allowedPrefix && !strings.HasPrefix(path, allowedPrefix+string(filepath.Separator)) {
					violations = append(violations, strings.TrimPrefix(path, root+string(filepath.Separator)))
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("github.com/cube2222/octosql imported outside the adapter boundary:\n  %s",
			strings.Join(violations, "\n  "))
	}
}
