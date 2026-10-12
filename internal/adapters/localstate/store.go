package localstate

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/mj8724/gameserver/internal/domain"
)

var instanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Store persists each instance below <dataRoot>/servers/<id>/state. If the new
// state file is absent, Load imports the legacy sibling instance.json read-only.
// It never creates or modifies the repository's data directory during Load.
type Store struct {
	dataRoot string
	defaults domain.InstanceState
	ops      fileOps

	mu      sync.Mutex
	secrets map[domain.InstanceID]string
}

// NewStore creates a store rooted at the configured data root. defaults is
// copied and completed with the legacy-compatible defaults for fields missing
// from it.
func NewStore(dataRoot string, defaults domain.InstanceState) (*Store, error) {
	return newStore(dataRoot, defaults, osFileOps{})
}

func newStore(dataRoot string, defaults domain.InstanceState, ops fileOps) (*Store, error) {
	if strings.TrimSpace(dataRoot) == "" || strings.ContainsRune(dataRoot, '\x00') {
		return nil, errors.New("valid data root is required")
	}
	root, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve data root: %w", err)
	}
	defaults = cloneState(defaults)
	if defaults.Variables == nil {
		defaults.Variables = map[string]any{}
	}
	if defaults.Ports == nil {
		defaults.Ports = map[string]int{}
	}
	return &Store{
		dataRoot: root,
		defaults: defaults,
		ops:      ops,
		secrets:  make(map[domain.InstanceID]string),
	}, nil
}

// DefaultState returns the legacy initial state for an instance. The admin
// password is generated with crypto/rand and is never logged by this package.
func DefaultState(id domain.InstanceID) (domain.InstanceState, error) {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return domain.InstanceState{}, fmt.Errorf("generate initial admin password: %w", err)
	}
	return domain.InstanceState{
		ID:         id,
		Name:       "Project Zomboid Dedicated Server",
		TemplateID: "project_zomboid",
		Variables: map[string]any{
			"SERVER_NAME":       "servertest",
			"SERVER_PASSWORD":   "",
			"ADMIN_PASSWORD":    base64.RawURLEncoding.EncodeToString(secret),
			"MAX_PLAYERS":       16,
			"PVP_ENABLED":       true,
			"PUBLIC_SERVER":     true,
			"OPEN_REGISTRATION": true,
			"PAUSE_EMPTY":       true,
		},
		Ports: map[string]int{
			"SERVER_PORT": 16261,
			"DIRECT_PORT": 16262,
		},
	}, nil
}

// Paths returns the current and legacy state paths for an instance. It is
// exposed for composition and diagnostics; callers must not log file contents.
func (s *Store) Paths(id domain.InstanceID) (current, legacy string, err error) {
	if !validInstanceID(id) {
		return "", "", errors.New("invalid instance id")
	}
	instanceRoot := filepath.Join(s.dataRoot, "servers", string(id))
	return filepath.Join(instanceRoot, "state", "instance.json"), filepath.Join(instanceRoot, "instance.json"), nil
}

// Load loads state and applies the legacy one-level nested default merge. The
// legacy source is never changed or removed.
func (s *Store) Load(ctx context.Context, id domain.InstanceID) (domain.InstanceState, error) {
	if err := ctx.Err(); err != nil {
		return domain.InstanceState{}, err
	}
	current, legacy, err := s.Paths(id)
	if err != nil {
		return domain.InstanceState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := current
	currentInfo, statErr := s.ops.Lstat(current)
	if statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return domain.InstanceState{}, fmt.Errorf("inspect state file: %w", statErr)
		}
		path = legacy
	} else if currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.Mode().IsRegular() {
		return domain.InstanceState{}, errors.New("state path is not a regular file; recovery required")
	}

	doc, err := s.defaultDocument(id)
	if err != nil {
		return domain.InstanceState{}, err
	}
	if info, err := s.ops.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return domain.InstanceState{}, errors.New("state path is not a regular file; recovery required")
		}
		data, err := s.ops.ReadFile(path)
		if err != nil {
			return domain.InstanceState{}, fmt.Errorf("read state file: %w", err)
		}
		var saved map[string]any
		if err := json.Unmarshal(data, &saved); err != nil {
			return domain.InstanceState{}, errors.New("state file is invalid JSON; recovery required")
		}
		if saved == nil {
			return domain.InstanceState{}, errors.New("state file must contain a JSON object; recovery required")
		}
		if savedID, ok := saved["instance_id"].(string); ok && savedID != "" && savedID != string(id) {
			return domain.InstanceState{}, errors.New("state file instance id does not match its path; recovery required")
		}
		doc = mergeDocument(doc, saved)
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.InstanceState{}, fmt.Errorf("inspect state file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.InstanceState{}, err
	}
	state, err := decodeState(doc, id)
	if err != nil {
		return domain.InstanceState{}, err
	}
	// Load performs no mkdir/chmod and never promotes the legacy source.
	return state, nil
}

// Save atomically writes the state beneath state/, retaining unknown legacy
// top-level fields loaded earlier or still present in the current/legacy JSON.
func (s *Store) Save(ctx context.Context, state domain.InstanceState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validInstanceID(state.ID) {
		return errors.New("invalid instance id")
	}
	current, legacy, err := s.Paths(state.ID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	base, err := s.defaultDocument(state.ID)
	if err != nil {
		return err
	}
	base, err = s.readExistingDocument(current, legacy, base)
	if err != nil {
		return err
	}
	updated, err := mergeState(base, state)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')

	instanceRoot := filepath.Join(s.dataRoot, "servers", string(state.ID))
	serversDir := filepath.Dir(instanceRoot)
	stateDir := filepath.Dir(current)
	if err := s.ensureSecureDirectory(serversDir); err != nil {
		return err
	}
	if err := s.ensureSecureDirectory(instanceRoot); err != nil {
		return err
	}
	if err := s.ensureSecureDirectory(stateDir); err != nil {
		return err
	}
	if err := s.ensureSecureFile(current); err != nil {
		return err
	}
	if err := s.ensureSecureFile(current + ".bak"); err != nil {
		return err
	}
	if err := atomicWriteFile(s.ops, current, data, 0o600); err != nil {
		return err
	}
	// atomicWriteFile has already read back and byte-compared the target.
	return nil
}

func (s *Store) defaultDocument(id domain.InstanceID) (map[string]any, error) {
	doc := stateDocument(s.defaults, id)
	variables, ok := doc["variables"].(map[string]any)
	if !ok {
		variables = map[string]any{}
		doc["variables"] = variables
	}
	secret, ok := variables["ADMIN_PASSWORD"].(string)
	if !ok || secret == "" {
		secret = s.secrets[id]
		if secret == "" {
			random := make([]byte, 24)
			if _, err := rand.Read(random); err != nil {
				return nil, fmt.Errorf("generate initial admin password: %w", err)
			}
			secret = base64.RawURLEncoding.EncodeToString(random)
			s.secrets[id] = secret
		}
		variables["ADMIN_PASSWORD"] = secret
	}
	return doc, nil
}

func (s *Store) readExistingDocument(current, legacy string, defaults map[string]any) (map[string]any, error) {
	path := current
	if _, err := s.ops.Lstat(current); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect state file: %w", err)
		}
		path = legacy
	}
	info, err := s.ops.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaults, nil
		}
		return nil, fmt.Errorf("inspect state file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("state path is not a regular file; recovery required")
	}
	data, err := s.ops.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil || saved == nil {
		return nil, errors.New("state file is invalid JSON; recovery required")
	}
	if savedID, ok := saved["instance_id"].(string); ok && savedID != "" && savedID != filepath.Base(filepath.Dir(filepath.Dir(current))) {
		return nil, errors.New("state file instance id does not match its path; recovery required")
	}
	return mergeDocument(defaults, saved), nil
}

func (s *Store) ensureSecureDirectory(path string) error {
	if err := s.ops.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	info, err := s.ops.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect state directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("state path is not a real directory: %q", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		return fmt.Errorf("state directory permissions must be 0700: %q", path)
	}
	return nil
}

func (s *Store) ensureSecureFile(path string) error {
	info, err := s.ops.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect state file permissions: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("state path is not a regular file: %q", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return fmt.Errorf("state file permissions must be 0600: %q", path)
	}
	return nil
}

func validInstanceID(id domain.InstanceID) bool { return instanceIDPattern.MatchString(string(id)) }

func stateDocument(defaults domain.InstanceState, id domain.InstanceID) map[string]any {
	state := cloneState(defaults)
	state.ID = id
	if state.Name == "" {
		state.Name = "Project Zomboid Dedicated Server"
	}
	if state.TemplateID == "" {
		state.TemplateID = "project_zomboid"
	}
	variables := map[string]any{
		"SERVER_NAME":       "servertest",
		"SERVER_PASSWORD":   "",
		"ADMIN_PASSWORD":    "",
		"MAX_PLAYERS":       16,
		"PVP_ENABLED":       true,
		"PUBLIC_SERVER":     true,
		"OPEN_REGISTRATION": true,
		"PAUSE_EMPTY":       true,
	}
	for key, value := range state.Variables {
		variables[key] = value
	}
	ports := map[string]int{"SERVER_PORT": 16261, "DIRECT_PORT": 16262}
	for key, value := range state.Ports {
		ports[key] = value
	}
	state.Variables = variables
	state.Ports = ports
	return map[string]any{
		"instance_id": state.ID,
		"name":        state.Name,
		"template_id": state.TemplateID,
		"variables":   state.Variables,
		"ports":       intMapAsAny(state.Ports),
		"quota_gb":    30.0,
		"mods": map[string]any{
			"workshop_ids": []any{},
			"mod_names":    []any{},
		},
		"billing": map[string]any{
			"status":           "ACTIVE",
			"package_name":     "帕鲁/僵尸生存 4核16G 畅玩版",
			"expire_days_left": 28,
		},
	}
}

func mergeDocument(defaults, saved map[string]any) map[string]any {
	merged := cloneDocument(defaults)
	for key, value := range saved {
		if defaultMap, ok := merged[key].(map[string]any); ok {
			if savedMap, ok := value.(map[string]any); ok {
				for nestedKey, nestedValue := range savedMap {
					defaultMap[nestedKey] = nestedValue
				}
				merged[key] = defaultMap
				continue
			}
		}
		merged[key] = value
	}
	return merged
}

func decodeState(doc map[string]any, id domain.InstanceID) (domain.InstanceState, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return domain.InstanceState{}, fmt.Errorf("decode state: %w", err)
	}
	var state domain.InstanceState
	if err := json.Unmarshal(data, &state); err != nil {
		return domain.InstanceState{}, fmt.Errorf("decode state: %w", err)
	}
	if state.ID == "" {
		state.ID = id
	}
	if state.ID != id {
		return domain.InstanceState{}, errors.New("state file instance id does not match its path; recovery required")
	}
	if state.Variables == nil {
		state.Variables = map[string]any{}
	}
	if name, ok := state.Variables["SERVER_NAME"].(string); !ok || !serverNamePattern.MatchString(name) {
		state.Variables["SERVER_NAME"] = "servertest"
	}
	if state.Ports == nil {
		state.Ports = map[string]int{}
	}
	return state, nil
}

func mergeState(base map[string]any, state domain.InstanceState) (map[string]any, error) {
	if err := validateDomainState(state); err != nil {
		return nil, err
	}
	updated := cloneDocument(base)
	updated["instance_id"] = string(state.ID)
	updated["name"] = state.Name
	updated["template_id"] = state.TemplateID
	variables, _ := updated["variables"].(map[string]any)
	if variables == nil {
		variables = make(map[string]any)
	} else {
		variables = cloneAnyMap(variables)
	}
	for key, value := range state.Variables {
		variables[key] = cloneAny(value)
	}
	updated["variables"] = variables
	ports, _ := updated["ports"].(map[string]any)
	if ports == nil {
		ports = make(map[string]any)
	} else {
		ports = cloneAnyMap(ports)
	}
	for key, value := range state.Ports {
		ports[key] = value
	}
	updated["ports"] = ports
	if state.Mods != nil {
		mods, _ := updated["mods"].(map[string]any)
		if mods == nil {
			mods = make(map[string]any)
		} else {
			mods = cloneAnyMap(mods)
		}
		for key, value := range state.Mods {
			mods[key] = cloneAny(value)
		}
		updated["mods"] = mods
	}
	if state.Billing != nil {
		billing, _ := updated["billing"].(map[string]any)
		if billing == nil {
			billing = make(map[string]any)
		} else {
			billing = cloneAnyMap(billing)
		}
		for key, value := range state.Billing {
			billing[key] = cloneAny(value)
		}
		updated["billing"] = billing
	}
	if state.QuotaGB > 0 {
		updated["quota_gb"] = state.QuotaGB
	}
	return updated, nil
}

func validateDomainState(state domain.InstanceState) error {
	if !validInstanceID(state.ID) {
		return errors.New("invalid instance id")
	}
	if strings.ContainsRune(state.Name, '\x00') || strings.ContainsRune(state.TemplateID, '\x00') {
		return errors.New("invalid state text")
	}
	if state.Variables == nil || state.Ports == nil {
		return errors.New("state variables and ports must be non-nil")
	}
	if name, ok := state.Variables["SERVER_NAME"].(string); ok && !serverNamePattern.MatchString(name) {
		return errors.New("invalid server name")
	}
	return nil
}

func cloneState(state domain.InstanceState) domain.InstanceState {
	state.Variables = cloneAnyMap(state.Variables)
	state.Ports = cloneIntMap(state.Ports)
	state.Mods = cloneAnyMap(state.Mods)
	state.Billing = cloneAnyMap(state.Billing)
	return state
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = cloneAny(value)
		}
		return out
	default:
		return typed
	}
}

func cloneIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func intMapAsAny(input map[string]int) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneDocument(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	return cloneAnyMap(input)
}
