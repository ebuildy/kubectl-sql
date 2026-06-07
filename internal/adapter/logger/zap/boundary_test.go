package zap

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestZapImportBoundary enforces the hexagonal boundary: go.uber.org/zap may be
// imported only by the adapter package (internal/adapter/logger/zap) and the cmd
// composition root that wires it. Any other importer means the logging library
// has leaked across the port and the spec's isolation guarantee is broken.
func TestZapImportBoundary(t *testing.T) {
	// Walk up to the repo root (this test lives at internal/adapter/logger/zap).
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	allowedPrefixes := []string{
		filepath.Join(root, "internal", "adapter", "logger", "zap"),
		filepath.Join(root, "cmd"),
	}

	fset := token.NewFileSet()
	var violations []string

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip vendored / external / build dirs.
			base := d.Name()
			if base == "external" || base == "bin" || base == ".git" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil // unparseable files are not our concern here
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == "go.uber.org/zap" || strings.HasPrefix(p, "go.uber.org/zap/") {
				if !allowed(path, allowedPrefixes) {
					violations = append(violations, strings.TrimPrefix(path, root+string(filepath.Separator)))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("go.uber.org/zap imported outside the adapter/cmd boundary:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func allowed(path string, prefixes []string) bool {
	for _, pre := range prefixes {
		if strings.HasPrefix(path, pre+string(filepath.Separator)) || path == pre {
			return true
		}
	}
	return false
}
