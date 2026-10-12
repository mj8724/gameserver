// Command pzoptions turns the vendor's own configuration files into the
// declarative option catalogue used by the web console.
//
// It reads the archived vendor artifacts (never a running server), so the
// catalogue is reproducible offline and its provenance can be re-verified:
//
//	go run ./tools/pzoptions -ini <servertest.ini> -sandbox <servertest_SandboxVars.lua> \
//	    -template project_zomboid -build 42.21 -out catalogs/project_zomboid.options.yaml
//
// The tool never executes Lua: it only reads assignments.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type option struct {
	Target      string
	Key         string
	Path        string
	Type        string
	Secret      bool
	Default     string
	Group       string
	Description string
	Writable    string
	Restart     bool
}

// variableBoundKeys are written through the legacy variables path, so the
// catalogue marks them read-only here (single-writer rule, plan D-C).
var variableBoundKeys = map[string]string{
	"Public":        "PUBLIC_SERVER",
	"Password":      "SERVER_PASSWORD",
	"MaxPlayers":    "MAX_PLAYERS",
	"PVP":           "PVP_ENABLED",
	"Open":          "OPEN_REGISTRATION",
	"PauseEmpty":    "PAUSE_EMPTY",
	"DefaultPort":   "SERVER_PORT",
	"UDPPort":       "DIRECT_PORT",
	"WorkshopItems": "WORKSHOP_ITEMS(mods API)",
	"Mods":          "MODS(mods API)",
}

// secretKeys never echo their value back (plan D-F).
var secretKeys = map[string]bool{"Password": true, "RCONPassword": true}

var iniLine = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\s*=\s*(.*)$`)
var luaAssign = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9_]*)\s*=\s*(.+?),?\s*$`)

func main() {
	iniPath := flag.String("ini", "", "vendor servertest.ini")
	sandboxPath := flag.String("sandbox", "", "vendor servertest_SandboxVars.lua")
	templateID := flag.String("template", "project_zomboid", "template id")
	buildID := flag.String("build", "", "PZ build/version the artifacts came from")
	out := flag.String("out", "", "output catalogue path")
	flag.Parse()
	if *iniPath == "" || *sandboxPath == "" || *out == "" {
		fail("pzoptions: -ini, -sandbox and -out are required")
	}

	iniOptions, iniKeys, iniSHA, err := parseINI(*iniPath)
	if err != nil {
		fail("pzoptions: %v", err)
	}
	sandboxOptions, sandboxPaths, sandboxSHA, err := parseSandbox(*sandboxPath)
	if err != nil {
		fail("pzoptions: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fail("pzoptions: %v", err)
	}
	file, err := os.Create(*out)
	if err != nil {
		fail("pzoptions: %v", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)

	now := os.Getenv("PZOPTIONS_TIMESTAMP")
	if now == "" {
		now = "see docs/acceptance/evidence/M2-windows-vendor-config-extraction.md"
	}
	fmt.Fprintf(writer, "# 由 tools/pzoptions 从厂商配置生成，请勿手改。\n")
	fmt.Fprintf(writer, "# 生成命令: go run ./tools/pzoptions -ini <servertest.ini> -sandbox <servertest_SandboxVars.lua> -template %s -build %s -out %s\n",
		*templateID, *buildID, *out)
	fmt.Fprintf(writer, "schema_version: \"1.0\"\n")
	fmt.Fprintf(writer, "template_id: %q\n", *templateID)
	fmt.Fprintf(writer, "source:\n")
	fmt.Fprintf(writer, "  build_id: %q\n", *buildID)
	fmt.Fprintf(writer, "  extracted_at: %q\n", now)
	fmt.Fprintf(writer, "  files:\n")
	fmt.Fprintf(writer, "    - path: %q\n      sha256: %q\n", filepath.Base(*iniPath), iniSHA)
	fmt.Fprintf(writer, "    - path: %q\n      sha256: %q\n", filepath.Base(*sandboxPath), sandboxSHA)
	fmt.Fprintf(writer, "  command: %q\n", "cmd.exe /c StartServer64.bat \"-cachedir=...\" \"-servername=servertest\"; see extraction record")
	fmt.Fprintf(writer, "options:\n")

	all := append(iniOptions, sandboxOptions...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Target != all[j].Target {
			return all[i].Target < all[j].Target
		}
		return all[i].Key+all[i].Path < all[j].Key+all[j].Path
	})
	for _, opt := range all {
		fmt.Fprintf(writer, "  - target: %q\n", opt.Target)
		if opt.Key != "" {
			fmt.Fprintf(writer, "    key: %q\n", opt.Key)
		}
		if opt.Path != "" {
			fmt.Fprintf(writer, "    path: %q\n", opt.Path)
		}
		fmt.Fprintf(writer, "    label: %q\n", opt.Key+opt.Path)
		fmt.Fprintf(writer, "    type: %q\n", opt.Type)
		fmt.Fprintf(writer, "    secret: %t\n", opt.Secret)
		fmt.Fprintf(writer, "    default: %q\n", opt.Default)
		fmt.Fprintf(writer, "    group: %q\n", opt.Group)
		if opt.Description != "" {
			fmt.Fprintf(writer, "    description: %q\n", opt.Description)
		}
		fmt.Fprintf(writer, "    requires_restart: %t\n", opt.Restart)
		fmt.Fprintf(writer, "    writable: %q\n", opt.Writable)
		fmt.Fprintf(writer, "    clearable: %t\n", !opt.Secret)
	}
	if err := writer.Flush(); err != nil {
		fail("pzoptions: %v", err)
	}

	// Set equality: every vendor key must be represented exactly once.
	seenINI := map[string]bool{}
	seenSandbox := map[string]bool{}
	for _, opt := range all {
		if opt.Target == "ini" {
			if seenINI[opt.Key] {
				fail("pzoptions: duplicate ini option %q", opt.Key)
			}
			seenINI[opt.Key] = true
		} else {
			if seenSandbox[opt.Path] {
				fail("pzoptions: duplicate sandbox path %q", opt.Path)
			}
			seenSandbox[opt.Path] = true
		}
	}
	if missing := difference(iniKeys, seenINI); len(missing) > 0 {
		fail("pzoptions: ini keys missing from catalogue: %v", missing)
	}
	if missing := difference(sandboxPaths, seenSandbox); len(missing) > 0 {
		fail("pzoptions: sandbox paths missing from catalogue: %v", missing)
	}
	fmt.Printf("OK ini_keys=%d sandbox_paths=%d options=%d out=%s\n", len(iniKeys), len(sandboxPaths), len(all), *out)
}

func parseINI(path string) ([]option, map[string]bool, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	sum := sha256.Sum256(raw)
	keys := map[string]bool{}
	var options []option
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := iniLine.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		key, value := match[1], strings.TrimSpace(match[2])
		keys[key] = true
		writable := "rw"
		if bound, ok := variableBoundKeys[key]; ok {
			writable = "ro"
			_ = bound
		}
		options = append(options, option{
			Target:   "ini",
			Key:      key,
			Type:     inferType(value, key),
			Secret:   secretKeys[key],
			Default:  redactSecret(value, key),
			Group:    iniGroup(key),
			Writable: writable,
			Restart:  true,
		})
	}
	return options, keys, hex.EncodeToString(sum[:]), scanner.Err()
}

func parseSandbox(path string) ([]option, map[string]bool, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	sum := sha256.Sum256(raw)
	paths := map[string]bool{}
	var options []option
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "--") {
			continue
		}
		match := luaAssign.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		name, value := match[1], strings.TrimSpace(match[2])
		if name == "SandboxVars" || name == "VERSION" {
			continue
		}
		if paths[name] {
			continue
		}
		paths[name] = true
		options = append(options, option{
			Target:   "sandboxvars",
			Path:     name,
			Type:     inferType(value, name),
			Secret:   secretKeys[name],
			Default:  redactSecret(value, name),
			Group:    "sandbox",
			Writable: "rw",
			Restart:  true,
		})
	}
	return options, paths, hex.EncodeToString(sum[:]), scanner.Err()
}

func inferType(value, key string) string {
	lower := strings.ToLower(value)
	switch {
	case lower == "true" || lower == "false":
		return "bool"
	case regexp.MustCompile(`^-?[0-9]+$`).MatchString(value):
		return "int"
	case regexp.MustCompile(`^-?[0-9]*\.[0-9]+$`).MatchString(value):
		return "float"
	case strings.HasPrefix(value, "\""):
		return "string"
	default:
		if strings.Contains(key, "Password") {
			return "string"
		}
		return "string"
	}
}

func redactSecret(value, key string) string {
	if secretKeys[key] {
		return ""
	}
	// Lua string literals are reported without their surrounding quotes.
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value[1 : len(value)-1]
	}
	return value
}

func iniGroup(key string) string {
	switch {
	case strings.Contains(key, "Port") || strings.Contains(key, "IP") || strings.Contains(key, "Steam"):
		return "network"
	case strings.Contains(key, "Password") || strings.Contains(key, "RCON") || strings.Contains(key, "Admin"):
		return "access"
	case strings.Contains(key, "Player") || strings.Contains(key, "PVP") || strings.Contains(key, "Safety"):
		return "players"
	default:
		return "server"
	}
}

func difference(want map[string]bool, got map[string]bool) []string {
	var missing []string
	for key := range want {
		if !got[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
