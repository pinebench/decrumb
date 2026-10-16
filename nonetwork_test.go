package decrumb

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestNoNetworkImports enforces spec §11: no package built into the binary
// (cmd/ and internal/) may import a networking package, directly or through
// another package. scripts/ is exempt: those are dev-time tools.
//
// It asks the go tool for the full dependency list, so packages added
// later are covered automatically.
func TestNoNetworkImports(t *testing.T) {
	deps := goListDeps(t, "./cmd/...", "./internal/...")

	if !slices.Contains(deps, "github.com/pinebench/decrumb/cmd/decrumb") {
		t.Fatalf("go list did not report cmd/decrumb; got %v", deps)
	}
	for _, pkg := range deps {
		if isNetworkPackage(pkg) {
			t.Errorf("binary depends on networking package %q", pkg)
		}
	}
}

// TestNoNetworkImportsDetects makes sure the check above would actually
// catch something: the update script uses net/http on purpose.
func TestNoNetworkImportsDetects(t *testing.T) {
	deps := goListDeps(t, "./scripts/update-rules")
	if !slices.ContainsFunc(deps, isNetworkPackage) {
		t.Error("expected scripts/update-rules to be flagged as using the network")
	}
}

func TestIsNetworkPackage(t *testing.T) {
	tests := []struct {
		pkg  string
		want bool
	}{
		{"net", true},
		{"net/http", true},
		{"net/http/httptrace", true},
		{"net/netip", true},
		{"net/rpc", true},
		{"crypto/tls", true},
		{"golang.org/x/net/html", true},
		{"net/url", false},
		{"network", false},
		{"encoding/json", false},
		{"github.com/pinebench/decrumb/internal/rules", false},
	}
	for _, tt := range tests {
		if got := isNetworkPackage(tt.pkg); got != tt.want {
			t.Errorf("isNetworkPackage(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

// isNetworkPackage reports whether pkg is a networking package.
// net/url is allowed: it only parses strings.
func isNetworkPackage(pkg string) bool {
	switch {
	case pkg == "net/url":
		return false
	case pkg == "net", strings.HasPrefix(pkg, "net/"):
		return true
	case pkg == "crypto/tls":
		return true
	case pkg == "golang.org/x/net", strings.HasPrefix(pkg, "golang.org/x/net/"):
		return true
	}
	return false
}

// goListDeps returns the import paths of the given packages and everything
// they depend on (test-only imports are not included).
func goListDeps(t *testing.T, patterns ...string) []string {
	t.Helper()
	args := append([]string{"list", "-deps", "-f", "{{.ImportPath}}"}, patterns...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	return strings.Fields(string(out))
}
