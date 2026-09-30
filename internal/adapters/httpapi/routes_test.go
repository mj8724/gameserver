package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// routePattern matches every mux registration in this package's source.
var routePattern = regexp.MustCompile(`s\.mux\.HandleFunc\("([A-Z]+) (/[^"]+)"`)

// Every registered route must also pass the request-path whitelist and method
// table. Without this, a new endpoint looks registered but answers 404/405
// through the middleware — a defect that shipped twice (instances, mods
// download) before being caught by live testing.
func TestEveryRegisteredRouteIsWhitelisted(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	matches := routePattern.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("no routes found: the pattern or the registration style changed")
	}
	for _, match := range matches {
		method, pattern := match[1], match[2]
		if pattern == "/{$}" {
			// The root wildcard pattern is the exact "/" path, which the
			// whitelist handles in its own early branch.
			pattern = "/"
		}
		if !knownPath(pattern) {
			t.Errorf("route %s %s is not in isKnownRequestPath (middleware would 404 it)", method, pattern)
			continue
		}
		methods := allowedMethods(pattern)
		allowed := false
		for _, candidate := range methods {
			if candidate == method {
				allowed = true
			}
		}
		if !allowed {
			t.Errorf("route %s %s: method table allows %v", method, pattern, methods)
		}
	}
	// Parameterised patterns must be reachable through their item helpers.
	for _, pair := range [][2]string{
		{"DELETE", "/api/instances/valheim_01"},
		{"DELETE", "/api/server/mods/281990"},
		{"POST", "/api/nodes/node-x/revoke"},
	} {
		if !knownPath(pair[1]) {
			t.Errorf("%s %s must be reachable", pair[0], pair[1])
		}
		allowed := false
		for _, candidate := range allowedMethods(pair[1]) {
			if candidate == pair[0] {
				allowed = true
			}
		}
		if !allowed {
			t.Errorf("%s %s must be allowed by the method table", pair[0], pair[1])
		}
	}
	if !strings.Contains(string(source), "/api/server/mods/download") {
		t.Fatal("mods download route disappeared")
	}
}
