package archtest

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// routePattern matches every mux registration in the HTTP adapter.
var routePattern = regexp.MustCompile(`s\.mux\.HandleFunc\("([A-Z]+) (/[^"]+)"`)

// Every registered HTTP route must also be reachable through the request-path
// whitelist and method table. Without this guard a new endpoint looks
// registered but answers 404/405 through the middleware — a defect that shipped
// twice (instances listing, mods download) before live testing caught it.
//
// This lives in archtest because it is a cross-layer consistency invariant and
// because the adapter layer may not import os.
func TestEveryRegisteredHTTPRouteIsReachable(t *testing.T) {
	source, err := os.ReadFile("../adapters/httpapi/server.go")
	if err != nil {
		t.Fatalf("read httpapi source: %v", err)
	}
	// The whitelist helpers live in the same file; parse them here as text so
	// this package keeps no dependency on the adapter layer.
	whitelisted := regexp.MustCompile(`(?s)func knownPath\(requestPath string\) bool \{(.*?)\n\}`).FindStringSubmatch(string(source))
	methodTable := regexp.MustCompile(`(?s)func allowedMethods\(requestPath string\) \[\]string \{(.*?)\n\}`).FindStringSubmatch(string(source))
	if whitelisted == nil || methodTable == nil {
		t.Fatal("whitelist or method table not found: the guard must be updated with the adapter")
	}
	matches := routePattern.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("no routes found: the registration style changed")
	}
	for _, match := range matches {
		method, pattern := match[1], match[2]
		if pattern == "/{$}" {
			pattern = "/" // the exact-root pattern is the "/" path
		}
		if strings.HasPrefix(pattern, "/static/") {
			// Covered by the static predicate, asserted separately below.
			continue
		}
		if !whitelistCovers(whitelisted[1], pattern) {
			t.Errorf("route %s %s is not whitelisted (middleware would answer 404)", method, pattern)
		}
		if !methodTableCovers(methodTable[1], pattern, method) {
			t.Errorf("route %s %s is not allowed by the method table", method, pattern)
		}
	}
	// Static asset routes are handled by the static-path predicate.
	for _, match := range matches {
		pattern := match[2]
		if strings.HasPrefix(pattern, "/static/") && !whitelistCovers(whitelisted[1], pattern) {
			if !strings.Contains(whitelisted[1], "isStaticRequestPath") {
				t.Errorf("static route %s is not covered", pattern)
			}
		}
	}
	for _, pair := range [][2]string{
		{"DELETE", "/api/instances/valheim_01"},
		{"DELETE", "/api/server/mods/281990"},
		{"POST", "/api/nodes/node-x/revoke"},
		{"POST", "/api/server/mods/download"},
	} {
		if !whitelistCovers(whitelisted[1], pair[1]) {
			t.Errorf("%s %s must be reachable", pair[0], pair[1])
		}
	}
	if !strings.Contains(string(source), "/api/server/mods/download") {
		t.Fatal("mods download route disappeared")
	}
}

// whitelistCovers reports whether a literal path (or a parameterised item path
// recognised by the item helpers) passes the whitelist body.
func whitelistCovers(body, path string) bool {
	if strings.Contains(body, `"`+path+`"`) {
		return true
	}
	// Prefix-based item paths handled by the helpers.
	for _, prefix := range []string{"/api/instances/", "/api/server/mods/", "/api/nodes/"} {
		if strings.HasPrefix(path, prefix) {
			return strings.Contains(body, "isInstanceItemPath") || strings.Contains(body, "isModItemPath") || strings.Contains(body, "isNodeActionPath")
		}
	}
	if path == "/" {
		return strings.Contains(body, `requestPath == "/"`)
	}
	return false
}

// methodTableCovers reports whether the method table body mentions the path and
// the method.
func methodTableCovers(body, path, method string) bool {
	if !strings.Contains(body, `"`+path+`"`) {
		// Parameterised routes fall through to the helper cases.
		if strings.HasPrefix(path, "/api/instances/") || strings.HasPrefix(path, "/api/server/mods/") || strings.HasPrefix(path, "/api/nodes/") {
			return strings.Contains(body, "http.MethodDelete") || strings.Contains(body, "http.MethodPost")
		}
		return false
	}
	return strings.Contains(body, "http.Method"+strings.Title(strings.ToLower(method))) ||
		strings.Contains(body, "http.Method"+method[:1]+strings.ToLower(method[1:]))
}
