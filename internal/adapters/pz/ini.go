package pz

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/mj8724/gameserver/internal/domain"
)

var instancePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var defaultINIValues = map[string]string{
	"Public":                    "true",
	"Password":                  "",
	"Open":                      "true",
	"PVP":                       "true",
	"PauseEmpty":                "true",
	"DefaultPort":               "16261",
	"UDPPort":                   "16262",
	"MaxPlayers":                "16",
	"Mods":                      "",
	"WorkshopItems":             "",
	"PingLimit":                 "400",
	"AutoCreateUserInWhiteList": "true",
	"DisplayUserName":           "true",
	"SpawnItems":                "",
	"RCONPort":                  "27015",
	"RCONPassword":              "",
}

// isManagedINIKey is the code-owned allow-list. Other entries in a PZ INI are
// preserved byte-for-byte and cannot be overwritten via Apply.
func isManagedINIKey(key string) bool {
	switch key {
	case "Public", "Password", "Open", "PVP", "PauseEmpty", "DefaultPort", "UDPPort", "MaxPlayers", "Mods", "WorkshopItems":
		return true
	default:
		return false
	}
}

type nameResolver func(context.Context, domain.InstanceID) (string, error)

type iniTempFile interface {
	Name() string
	Write([]byte) (int, error)
	Sync() error
	Chmod(os.FileMode) error
	Close() error
}

type iniFileOps interface {
	MkdirAll(string, os.FileMode) error
	Lstat(string) (os.FileInfo, error)
	ReadFile(string) ([]byte, error)
	CreateTemp(string, string) (iniTempFile, error)
	Rename(string, string) error
	Remove(string) error
	SyncDir(string) error
}

type osINIFileOps struct{}

func (osINIFileOps) MkdirAll(path string, mode os.FileMode) error { return os.MkdirAll(path, mode) }
func (osINIFileOps) Lstat(path string) (os.FileInfo, error)       { return os.Lstat(path) }
func (osINIFileOps) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (osINIFileOps) CreateTemp(dir, pattern string) (iniTempFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osINIFileOps) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (osINIFileOps) Remove(path string) error             { return os.Remove(path) }

// SyncDir flushes a directory entry; on Windows this is a documented no-op
// because FlushFileBuffers on a directory handle fails with ERROR_ACCESS_DENIED
// and the rename plus the file flush already provide durability.
func (osINIFileOps) SyncDir(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// NameResolver obtains SERVER_NAME from the application's state store. The
// resolver is injected so this adapter does not depend on another adapter.
type NameResolver func(context.Context, domain.InstanceID) (string, error)

// Config stores PZ INI files in <dataRoot>/servers/<instance>/Zomboid/Server.
// It implements ports.GameConfig without interpreting template strings.
type Config struct {
	dataRoot string
	resolve  NameResolver
	ops      iniFileOps
	seedPath string
}

// NewConfig constructs an INI adapter rooted at dataRoot. If resolve is nil,
// the legacy default server name "servertest" is used; callers with persisted
// non-default names should provide a resolver.
func NewConfig(dataRoot string, resolve NameResolver) (*Config, error) {
	if strings.TrimSpace(dataRoot) == "" || strings.ContainsRune(dataRoot, '\x00') {
		return nil, errors.New("valid data root is required")
	}
	root, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve data root: %w", err)
	}
	return &Config{dataRoot: root, resolve: resolve, ops: osINIFileOps{}}, nil
}

func newConfig(dataRoot string, resolve NameResolver, ops iniFileOps) (*Config, error) {
	config, err := NewConfig(dataRoot, resolve)
	if err != nil {
		return nil, err
	}
	config.ops = ops
	return config, nil
}

// Path validates both identifiers before constructing a state-root-relative
// INI path. The filename is always exactly <SERVER_NAME>.ini.
func (c *Config) Path(id domain.InstanceID, serverName string) (string, error) {
	if !instancePattern.MatchString(string(id)) {
		return "", errors.New("invalid instance id")
	}
	if !serverNamePattern.MatchString(serverName) {
		return "", errors.New("invalid server name")
	}
	return filepath.Join(c.dataRoot, "servers", string(id), "Zomboid", "Server", serverName+".ini"), nil
}

// Read implements ports.GameConfig. It returns a copy of the legacy defaults
// overlaid with values read from the current INI (last duplicate key wins).
func (c *Config) Read(ctx context.Context, id domain.InstanceID) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := c.serverName(ctx, id)
	if err != nil {
		return nil, err
	}
	path, err := c.Path(id, name)
	if err != nil {
		return nil, err
	}
	result := cloneStrings(defaultINIValues)
	data, err := c.ops.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read PZ INI: %w", err)
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		text := strings.TrimSuffix(string(line), "\r")
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result, nil
}

// Apply implements ports.GameConfig. Unknown keys are rejected rather than
// written, and values containing line breaks/NUL are refused to prevent INI
// injection. The supplied map only changes the listed keys.
func (c *Config) Apply(ctx context.Context, id domain.InstanceID, updates map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := c.serverName(ctx, id)
	if err != nil {
		return err
	}
	if candidate, ok := updates["SERVER_NAME"]; ok {
		name = candidate
	}
	path, err := c.Path(id, name)
	if err != nil {
		return err
	}
	managedUpdates := make(map[string]string, len(updates))
	for key, value := range updates {
		if key == "SERVER_NAME" {
			if !serverNamePattern.MatchString(value) {
				return errors.New("invalid server name")
			}
			continue // path selector, never an INI setting
		}
		if !isManagedINIKey(key) {
			return fmt.Errorf("unmanaged PZ INI key %q", key)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid value for PZ INI key %q", key)
		}
		managedUpdates[key] = value
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(managedUpdates) == 0 {
		return nil
	}
	return c.applyFile(path, managedUpdates)
}

// ApplyNamed applies keys to an explicitly named server INI. It is useful for
// adapters which already have a validated state snapshot at hand.
func (c *Config) ApplyNamed(ctx context.Context, id domain.InstanceID, serverName string, updates map[string]string) error {
	return c.ApplyNamedAllowed(ctx, id, serverName, updates, isManagedINIKey)
}

// ApplyNamedAllowed applies updates whose keys are authorised by the caller. The
// legacy variables path passes the code-owned allow-list; the options path
// passes an allow function derived from the validated option catalogue, so the
// writable set stays explicit and reviewable instead of implicit.
func (c *Config) ApplyNamedAllowed(ctx context.Context, id domain.InstanceID, serverName string,
	updates map[string]string, allow func(string) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if allow == nil {
		return errors.New("an INI key authoriser is required")
	}
	path, err := c.Path(id, serverName)
	if err != nil {
		return err
	}
	managedUpdates := make(map[string]string, len(updates))
	for key, value := range updates {
		if !allow(key) {
			return fmt.Errorf("unmanaged PZ INI key %q", key)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid value for PZ INI key %q", key)
		}
		managedUpdates[key] = value
	}
	if len(managedUpdates) == 0 {
		return nil
	}
	return c.applyFile(path, managedUpdates)
}

func (c *Config) serverName(ctx context.Context, id domain.InstanceID) (string, error) {
	if !instancePattern.MatchString(string(id)) {
		return "", errors.New("invalid instance id")
	}
	if c.resolve == nil {
		return "servertest", nil
	}
	name, err := c.resolve(ctx, id)
	if err != nil {
		return "", err
	}
	if !serverNamePattern.MatchString(name) {
		return "", errors.New("invalid server name")
	}
	return name, nil
}

func (c *Config) applyFile(path string, updates map[string]string) error {
	if err := c.ensureSecurePath(path); err != nil {
		return err
	}
	data, err := c.ops.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return fmt.Errorf("read PZ INI before update: %w", err)
	}
	updated := updateINILines(data, updates)
	if err := atomicINIWrite(c.ops, path, updated); err != nil {
		return err
	}
	return nil
}

func updateINILines(data []byte, updates map[string]string) []byte {
	if len(updates) == 0 {
		return append([]byte(nil), data...)
	}
	seen := make(map[string]bool, len(updates))
	lines := bytes.SplitAfter(data, []byte("\n"))
	var out bytes.Buffer
	for _, raw := range lines {
		if len(raw) == 0 {
			continue
		}
		line := raw
		ending := []byte("\n")
		if bytes.HasSuffix(raw, []byte("\r\n")) {
			ending = []byte("\r\n")
			line = raw[:len(raw)-2]
		} else if bytes.HasSuffix(raw, []byte("\n")) {
			line = raw[:len(raw)-1]
		} else {
			ending = nil
		}
		trimmed := strings.TrimSpace(string(line))
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if key, _, found := strings.Cut(trimmed, "="); found {
				key = strings.TrimSpace(key)
				if value, ok := updates[key]; ok {
					fmt.Fprintf(&out, "%s=%s", key, value)
					out.Write(ending)
					seen[key] = true
					continue
				}
			}
		}
		out.Write(raw)
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if out.Len() > 0 && !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "%s=%s\n", key, updates[key])
	}
	return out.Bytes()
}

func (c *Config) ensureSecurePath(path string) error {
	serverDir := filepath.Join(c.dataRoot, "servers")
	relative, err := filepath.Rel(serverDir, path)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("PZ config path escaped the configured data root")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 4 || parts[0] == "" || parts[1] != "Zomboid" || parts[2] != "Server" {
		return errors.New("PZ config path is not canonical")
	}
	instanceDir := filepath.Join(serverDir, parts[0])
	zomboidDir := filepath.Join(instanceDir, "Zomboid")
	iniDir := filepath.Join(zomboidDir, "Server")
	for _, dir := range []string{serverDir, instanceDir, zomboidDir, iniDir} {
		if err := c.ops.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create PZ config directory: %w", err)
		}
		info, err := c.ops.Lstat(dir)
		if err != nil {
			return fmt.Errorf("inspect PZ config directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("PZ config path is not a real directory: %q", dir)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			return fmt.Errorf("PZ config directory permissions must be 0700: %q", dir)
		}
	}
	info, err := c.ops.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect PZ INI permissions: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("PZ INI path is not a regular file: %q", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return errors.New("PZ INI permissions must be 0600")
	}
	backupInfo, backupErr := c.ops.Lstat(path + ".bak")
	if backupErr == nil {
		if backupInfo.Mode()&os.ModeSymlink != 0 || !backupInfo.Mode().IsRegular() {
			return errors.New("PZ INI backup path is not a regular file")
		}
		if runtime.GOOS != "windows" && backupInfo.Mode().Perm() != 0o600 {
			return errors.New("PZ INI backup permissions must be 0600")
		}
	} else if !errors.Is(backupErr, os.ErrNotExist) {
		return fmt.Errorf("inspect PZ INI backup permissions: %w", backupErr)
	}
	return nil
}

func atomicINIWrite(ops iniFileOps, target string, data []byte) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	tmp, err := ops.CreateTemp(dir, "."+base+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary PZ INI: %w", err)
	}
	tmpPath := tmp.Name()
	closed, renamed := false, false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		if !renamed {
			_ = ops.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("set PZ INI mode: %w", err)
	}
	if n, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary PZ INI: %w", err)
	} else if n != len(data) {
		return fmt.Errorf("write temporary PZ INI: %w", io.ErrShortWrite)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary PZ INI: %w", err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return fmt.Errorf("close temporary PZ INI: %w", err)
	}
	closed = true
	if err := durableINIBackup(ops, dir, base+".bak", target); err != nil {
		return err
	}
	if err := ops.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("replace PZ INI: %w", err)
	}
	renamed = true
	if err := ops.SyncDir(dir); err != nil {
		return fmt.Errorf("sync PZ INI directory: %w", err)
	}
	readback, err := ops.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read back PZ INI: %w", err)
	}
	if !bytes.Equal(readback, data) {
		return errors.New("read back PZ INI differs from committed content")
	}
	return nil
}

func durableINIBackup(ops iniFileOps, dir, backupName, target string) error {
	old, err := ops.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read prior PZ INI for backup: %w", err)
	}
	f, err := ops.CreateTemp(dir, "."+backupName+".tmp-")
	if err != nil {
		return fmt.Errorf("create PZ INI backup: %w", err)
	}
	tmpPath := f.Name()
	closed, renamed := false, false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		if !renamed {
			_ = ops.Remove(tmpPath)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("set PZ INI backup mode: %w", err)
	}
	if n, err := f.Write(old); err != nil {
		return fmt.Errorf("write PZ INI backup: %w", err)
	} else if n != len(old) {
		return fmt.Errorf("write PZ INI backup: %w", io.ErrShortWrite)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync PZ INI backup: %w", err)
	}
	if err := f.Close(); err != nil {
		closed = true
		return fmt.Errorf("close PZ INI backup: %w", err)
	}
	closed = true
	if err := ops.Rename(tmpPath, filepath.Join(dir, backupName)); err != nil {
		return fmt.Errorf("replace PZ INI backup: %w", err)
	}
	renamed = true
	if err := ops.SyncDir(dir); err != nil {
		return fmt.Errorf("sync PZ INI backup directory: %w", err)
	}
	return nil
}

func cloneStrings(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
