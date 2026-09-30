// Package instanceregistry stores the per-host instance registry and allocates
// port pairs for new instances (M5). It reuses the state store's atomic write
// and permission rules so a registry update is either fully applied or absent.
package instanceregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

const registryFile = "instances.json"

// Registry is a file-backed instance registry rooted at the data root.
type Registry struct {
	dataRoot string
	mu       sync.Mutex
}

// New creates a registry rooted at dataRoot.
func New(dataRoot string) (*Registry, error) {
	if strings.TrimSpace(dataRoot) == "" || strings.ContainsRune(dataRoot, '\x00') {
		return nil, errors.New("valid data root is required")
	}
	absolute, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, err
	}
	return &Registry{dataRoot: absolute}, nil
}

func (r *Registry) path() string { return filepath.Join(r.dataRoot, registryFile) }

func (r *Registry) read() ([]ports.InstanceRecord, error) {
	raw, err := os.ReadFile(r.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var records []ports.InstanceRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("parse instance registry: %w", err)
	}
	return records, nil
}

func (r *Registry) write(records []ports.InstanceRecord) error {
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	encoded, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.dataRoot, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(r.dataRoot, ".instances-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(append(encoded, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tempName, r.path()); err != nil {
		return err
	}
	// Re-read and compare: a registry update only counts once verified.
	written, err := os.ReadFile(r.path())
	if err != nil {
		return err
	}
	if string(written) != string(append(encoded, '\n')) {
		return errors.New("instance registry readback mismatch")
	}
	return nil
}

// List returns every registered instance plus instances discovered on disk
// (a legacy single-instance deployment is registered implicitly on read).
func (r *Registry) List(_ context.Context) ([]ports.InstanceRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	records, err := r.read()
	if err != nil {
		return nil, err
	}
	known := map[domain.InstanceID]bool{}
	for _, record := range records {
		known[record.ID] = true
	}
	discovered, err := r.discover()
	if err != nil {
		return nil, err
	}
	for _, record := range discovered {
		if !known[record.ID] {
			records = append(records, record)
			known[record.ID] = true
		}
	}
	return records, nil
}

// Get returns one record.
func (r *Registry) Get(ctx context.Context, id domain.InstanceID) (ports.InstanceRecord, error) {
	records, err := r.List(ctx)
	if err != nil {
		return ports.InstanceRecord{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return ports.InstanceRecord{}, ports.ErrInstanceUnknown
}

// Add registers a new instance; an existing id is refused (no silent overwrite).
func (r *Registry) Add(_ context.Context, record ports.InstanceRecord) error {
	if strings.TrimSpace(string(record.ID)) == "" {
		return errors.New("instance id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	records, err := r.read()
	if err != nil {
		return err
	}
	for _, existing := range records {
		if existing.ID == record.ID {
			return ports.ErrInstanceExists
		}
	}
	if record.DataRoot == "" {
		record.DataRoot = filepath.Join(r.dataRoot, "servers", string(record.ID))
	}
	return r.write(append(records, record))
}

// Remove drops a registry entry. Instance data is never deleted here.
func (r *Registry) Remove(_ context.Context, id domain.InstanceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	records, err := r.read()
	if err != nil {
		return err
	}
	kept := make([]ports.InstanceRecord, 0, len(records))
	found := false
	for _, record := range records {
		if record.ID == id {
			found = true
			continue
		}
		kept = append(kept, record)
	}
	if !found {
		return ports.ErrInstanceUnknown
	}
	return r.write(kept)
}

// discover finds instance directories that are not in the registry yet.
func (r *Registry) discover() ([]ports.InstanceRecord, error) {
	serversRoot := filepath.Join(r.dataRoot, "servers")
	entries, err := os.ReadDir(serversRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var discovered []ports.InstanceRecord
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "#") {
			continue
		}
		record := ports.InstanceRecord{
			ID:       domain.InstanceID(entry.Name()),
			DataRoot: filepath.Join(serversRoot, entry.Name()),
		}
		if raw, err := os.ReadFile(filepath.Join(record.DataRoot, "instance.json")); err == nil {
			var document struct {
				Name       string `json:"name"`
				TemplateID string `json:"template_id"`
			}
			if json.Unmarshal(raw, &document) == nil {
				record.Name = document.Name
				record.TemplateID = document.TemplateID
			}
		}
		discovered = append(discovered, record)
	}
	return discovered, nil
}

// PortAllocator allocates port pairs, avoiding ports used by other instances
// and ports the OS reports as taken.
type PortAllocator struct {
	Registry *Registry
	// Start is the first candidate port of a block scan.
	Start int
	// Span is how many consecutive ports are scanned per key.
	Span int
}

// Allocate returns free ports for the requested keys, pairwise offset so the
// primary and the direct port never collide.
func (a *PortAllocator) Allocate(ctx context.Context, primaryKey, directKey string) (map[string]int, error) {
	if a == nil || a.Registry == nil {
		return nil, errors.New("port allocator requires a registry")
	}
	used, err := a.usedPorts(ctx)
	if err != nil {
		return nil, err
	}
	start := a.Start
	if start <= 0 {
		start = 27015
	}
	span := a.Span
	if span <= 0 {
		span = 1000
	}
	result := map[string]int{}
	offset := 0
	for offset < span {
		primary := start + offset
		direct := primary + 1
		if !used[primary] && !used[direct] && portFree(primary) && portFree(direct) {
			result[primaryKey] = primary
			result[directKey] = direct
			return result, nil
		}
		offset += 2
	}
	return nil, errors.New("no free port pair found in the configured range")
}

func (a *PortAllocator) usedPorts(ctx context.Context) (map[int]bool, error) {
	records, err := a.Registry.List(ctx)
	if err != nil {
		return nil, err
	}
	used := map[int]bool{}
	for _, record := range records {
		raw, err := os.ReadFile(filepath.Join(record.DataRoot, "instance.json"))
		if err != nil {
			continue
		}
		var document struct {
			Ports map[string]int `json:"ports"`
		}
		if json.Unmarshal(raw, &document) != nil {
			continue
		}
		for _, port := range document.Ports {
			used[port] = true
		}
	}
	return used, nil
}

// portFree reports whether a UDP port can be bound right now.
func portFree(port int) bool {
	conn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

var _ ports.InstanceRegistry = (*Registry)(nil)
var _ ports.PortAllocator = (*PortAllocator)(nil)
