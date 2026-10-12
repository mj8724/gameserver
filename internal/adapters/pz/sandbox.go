package pz

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/mj8724/gameserver/internal/domain"
	"strings"
)

// sandboxBackupGenerations keeps more than one previous revision so a bad write
// discovered after a second write can still be rolled back.
const sandboxBackupGenerations = 3

var sandboxAssign = regexp.MustCompile(`^(\s*)([A-Za-z][A-Za-z0-9_]*)(\s*=\s*)(.*?)(,?)(\s*)$`)

var namePatternSandbox = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// SandboxVarsPath resolves the vendor sandbox file for one server name.
func SandboxVarsPath(dataRoot string, id string, serverName string) (string, error) {
	if !instancePattern.MatchString(id) {
		return "", errors.New("invalid instance id")
	}
	if !serverNamePattern.MatchString(serverName) {
		return "", errors.New("invalid server name")
	}
	return filepath.Join(dataRoot, "servers", id, "Zomboid", "Server", serverName+"_SandboxVars.lua"), nil
}

// ReadSandbox returns the top-level assignments of a sandbox file. Only the
// assignment text is read; the file is never executed.
func ReadSandbox(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		match := sandboxAssign.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if _, seen := values[match[2]]; !seen {
			values[match[2]] = match[4]
		}
	}
	return values, nil
}

// normalizeSandboxValue validates that a value can be written as a single Lua
// assignment token. Anything that cannot be represented verbatim is rejected
// (fail closed) instead of being silently rewritten.
func normalizeSandboxValue(value string, quoted bool) (string, error) {
	if strings.ContainsAny(value, "\x00\n\r") {
		return "", errors.New("sandbox value must not contain control characters")
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "\"\"", nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		if !strings.HasSuffix(trimmed, "\"") || len(trimmed) < 2 {
			return "", errors.New("sandbox string literal is not terminated")
		}
		if strings.Contains(trimmed[1:len(trimmed)-1], "\"") {
			return "", errors.New("sandbox string literal must not contain a quote")
		}
		return trimmed, nil
	}
	switch strings.ToLower(trimmed) {
	case "true", "false":
		return strings.ToLower(trimmed), nil
	}
	if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return trimmed, nil
	}
	// A bare word is a string literal: the catalogue already validated that the
	// option's declared type is string, and anything without a quote or control
	// character is safe to write as a quoted value (a payload such as
	// os.execute('x') becomes inert text rather than code).
	if !strings.Contains(trimmed, "\"") {
		return "\"" + trimmed + "\"", nil
	}
	return "", fmt.Errorf("sandbox value %q is neither a literal nor a number", value)
}

// ApplySandbox rewrites only the assigned keys, preserving every other line
// (comments included), and keeps several .bak generations. The written file is
// re-read and compared before the call reports success.
func ApplySandbox(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	quoted := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if match := sandboxAssign.FindStringSubmatch(line); match != nil {
			if _, seen := quoted[match[2]]; !seen {
				quoted[match[2]] = strings.HasPrefix(strings.TrimSpace(match[4]), "\"")
			}
		}
	}
	normalized := make(map[string]string, len(updates))
	for key, value := range updates {
		if !namePatternSandbox.MatchString(key) {
			return fmt.Errorf("invalid sandbox key %q", key)
		}
		text, err := normalizeSandboxValue(value, quoted[key])
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		normalized[key] = text
	}

	lines := strings.Split(string(raw), "\n")
	seen := map[string]bool{}
	for index, line := range lines {
		match := sandboxAssign.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		value, ok := normalized[match[2]]
		if !ok {
			continue
		}
		lines[index] = match[1] + match[2] + match[3] + value + match[5] + match[6]
		seen[match[2]] = true
	}
	missing := make([]string, 0, len(normalized))
	for key := range normalized {
		if !seen[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("sandbox file has no assignment for %s", strings.Join(missing, ", "))
	}
	output := strings.Join(lines, "\n")

	if err := rotateSandboxBackups(path); err != nil {
		return err
	}
	ops := osINIFileOps{}
	if err := atomicINIWrite(ops, path, []byte(output)); err != nil {
		return err
	}
	readback, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(readback) != output {
		return errors.New("sandbox readback does not match the written content")
	}
	return nil
}

// rotateSandboxBackups keeps <path>.bak1..N, shifting older copies down.
func rotateSandboxBackups(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for generation := sandboxBackupGenerations; generation >= 1; generation-- {
		source := fmt.Sprintf("%s.bak%d", path, generation)
		target := fmt.Sprintf("%s.bak%d", path, generation+1)
		if generation == sandboxBackupGenerations {
			_ = os.Remove(target)
			continue
		}
		if _, err := os.Stat(source); err == nil {
			if err := os.Rename(source, target); err != nil {
				return err
			}
		}
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak1", current, 0o600)
}

// sandboxSeedPath is the vendor-baseline file copied into an instance the first
// time its sandbox file is needed. PZ writes the runtime file itself, so a fresh
// instance has none until the game runs; without a seed the options path would
// have to fail closed on every sandbox key.
// sandboxPath resolves the sandbox file for this instance, honouring an
// explicit configuration home when the game keeps its own directory.
func (c *Config) sandboxPath(id domain.InstanceID, serverName string) (string, error) {
	if c.home != "" {
		if !instancePattern.MatchString(string(id)) || !serverNamePattern.MatchString(serverName) {
			return "", errors.New("invalid instance id or server name")
		}
		return filepath.Join(c.home, "Server", serverName+"_SandboxVars.lua"), nil
	}
	return SandboxVarsPath(c.dataRoot, string(id), serverName)
}

func (c *Config) sandboxSeedPath() string {
	return c.seedPath
}

// SetSandboxSeed registers the baseline file used to seed missing instances.
func (c *Config) SetSandboxSeed(path string) { c.seedPath = path }

// ensureSandboxFile creates the instance sandbox file from the seed when it is
// missing, using the same atomic write protocol as every other write.
func (c *Config) ensureSandboxFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	seed := c.sandboxSeedPath()
	if seed == "" {
		return nil
	}
	raw, err := os.ReadFile(seed)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicINIWrite(osINIFileOps{}, path, raw)
}
