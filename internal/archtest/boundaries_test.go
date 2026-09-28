package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type listedPackage struct {
	ImportPath   string   `json:"ImportPath"`
	ForTest      string   `json:"ForTest"`
	Imports      []string `json:"Imports"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
	Error        *struct {
		Err string `json:"Err"`
	} `json:"Error"`
}

func basePackagePath(pkg listedPackage) string {
	if pkg.ForTest != "" {
		return pkg.ForTest
	}
	if strings.HasSuffix(pkg.ImportPath, ".test") {
		return ""
	}
	if index := strings.Index(pkg.ImportPath, " ["); index >= 0 {
		return pkg.ImportPath[:index]
	}
	return pkg.ImportPath
}

func TestImportBoundaries(t *testing.T) {
	root := findModuleRoot(t)
	standard := standardLibraryPackages(t, root)
	cmd := exec.Command("go", "list", "-e", "-json", "-test", "./...")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, stderr.String())
	}

	decoder := json.NewDecoder(&stdout)
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		fromPath := basePackagePath(pkg)
		if fromPath == "" {
			continue
		}
		if pkg.Error != nil {
			t.Fatalf("go list error for %s: %s", pkg.ImportPath, pkg.Error.Err)
		}

		imports := append([]string{}, pkg.Imports...)
		imports = append(imports, pkg.TestImports...)
		imports = append(imports, pkg.XTestImports...)
		for _, imported := range imports {
			if err := importViolation(fromPath, imported, standard); err != nil {
				t.Errorf("%v", err)
			}
		}
	}
}

func TestRulesRejectForbiddenImports(t *testing.T) {
	cases := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{
			name:    "domain cannot import ports",
			from:    modulePrefix + "/internal/domain",
			to:      modulePrefix + "/internal/ports",
			wantErr: true,
		},
		{
			name:    "application cannot import adapter",
			from:    modulePrefix + "/internal/application",
			to:      modulePrefix + "/internal/adapters/process",
			wantErr: true,
		},
		{
			name:    "adapter cannot import application except httpapi",
			from:    modulePrefix + "/internal/adapters/process",
			to:      modulePrefix + "/internal/application",
			wantErr: true,
		},
		{
			name:    "httpapi can import application",
			from:    modulePrefix + "/internal/adapters/httpapi",
			to:      modulePrefix + "/internal/application",
			wantErr: false,
		},
		{
			name:    "application can import ports",
			from:    modulePrefix + "/internal/application",
			to:      modulePrefix + "/internal/ports",
			wantErr: false,
		},
		{
			name:    "domain cannot import os",
			from:    modulePrefix + "/internal/domain",
			to:      "os",
			wantErr: true,
		},
		{
			name:    "application cannot import net/http",
			from:    modulePrefix + "/internal/application",
			to:      "net/http",
			wantErr: true,
		},
		{
			name:    "httpapi cannot access filesystem directly",
			from:    modulePrefix + "/internal/adapters/httpapi",
			to:      "path/filepath",
			wantErr: true,
		},
		{
			name:    "httpapi can use net/http",
			from:    modulePrefix + "/internal/adapters/httpapi",
			to:      "net/http",
			wantErr: false,
		},
		{
			name:    "domain cannot import filesystem",
			from:    modulePrefix + "/internal/domain",
			to:      "os",
			wantErr: true,
		},
		{
			name:    "httpapi cannot access filesystem directly",
			from:    modulePrefix + "/internal/adapters/httpapi",
			to:      "path/filepath",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := importViolation(tc.from, tc.to, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("importViolation(%q, %q) error = %v, wantErr %t", tc.from, tc.to, err, tc.wantErr)
			}
		})
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod from %s", cwd)
		}
	}
}

func standardLibraryPackages(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	cmd := exec.Command("go", "list", "std")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list std: %v", err)
	}
	packages := make(map[string]struct{})
	for _, path := range strings.Fields(string(output)) {
		packages[path] = struct{}{}
	}
	return packages
}

func TestInjectedForbiddenImportFailsArchitectureCheck(t *testing.T) {
	root := findModuleRoot(t)
	standard := standardLibraryPackages(t, root)
	violation := filepath.Join(root, "internal", "adapters", "process", "zz_archtest_violation.go")
	if _, err := os.Stat(violation); err == nil {
		t.Fatalf("temporary violation file already exists: %s", violation)
	}
	content := []byte("package process\nimport _ \"github.com/mj8724/gameserver/internal/application\"\n")
	if err := os.WriteFile(violation, content, 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(violation); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove injected violation: %v", err)
		}
	}()

	cmd := exec.Command("go", "list", "-e", "-json", "-test", "./...")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list with injected import failed unexpectedly: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		fromPath := basePackagePath(pkg)
		if fromPath != modulePrefix+"/internal/adapters/process" {
			continue
		}
		for _, imported := range pkg.Imports {
			if err := importViolation(fromPath, imported, standard); err != nil {
				if strings.Contains(err.Error(), "forbidden import") {
					return
				}
				t.Fatalf("unexpected validation error: %v", err)
			}
		}
	}
	t.Fatal("architecture check accepted the injected forbidden import")
}
