// Package nodestate stores the local node identity, the peer registry and the
// durable task ledger on disk (M6). The private key is written 0600 and is
// never returned by any read path.
package nodestate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mj8724/gameserver/internal/ports"
)

const (
	identityFile = "node-identity.json"
	keyFile      = "node-key.ed25519"
	peersFile    = "nodes.json"
	ledgerFile   = "tasks.json"
)

// Store is a file-backed node store rooted at a directory.
type Store struct {
	root string
	mu   sync.Mutex
}

// New creates a node store, creating the root directory when missing.
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" || strings.ContainsRune(root, '\x00') {
		return nil, errors.New("node store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: absolute}, nil
}

func (s *Store) path(name string) string { return filepath.Join(s.root, name) }

func (s *Store) writeVerified(name string, payload []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(s.root, "."+name+".*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(payload); err != nil {
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
	if err := os.Chmod(tempName, mode); err != nil {
		return err
	}
	target := s.path(name)
	if err := os.Rename(tempName, target); err != nil {
		return err
	}
	written, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if string(written) != string(payload) {
		return fmt.Errorf("node store readback mismatch for %s", name)
	}
	return nil
}

// Identity returns the node identity, generating the keypair on first use.
func (s *Store) Identity(_ context.Context) (ports.NodeIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identityLocked()
}

func (s *Store) identityLocked() (ports.NodeIdentity, error) {
	raw, err := os.ReadFile(s.path(identityFile))
	if err == nil {
		var identity ports.NodeIdentity
		if err := json.Unmarshal(raw, &identity); err != nil {
			return ports.NodeIdentity{}, fmt.Errorf("parse node identity: %w", err)
		}
		identity.PrivateKeyPath = s.path(keyFile)
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ports.NodeIdentity{}, err
	}
	return s.generateLocked()
}

func (s *Store) generateLocked() (ports.NodeIdentity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return ports.NodeIdentity{}, err
	}
	sum := sha256.Sum256(publicKey)
	identity := ports.NodeIdentity{
		NodeID:         "node-" + hex.EncodeToString(sum[:6]),
		Fingerprint:    hex.EncodeToString(sum[:]),
		PublicKey:      base64.StdEncoding.EncodeToString(publicKey),
		CreatedAt:      time.Now().UTC(),
		PrivateKeyPath: s.path(keyFile),
	}
	// The secret is written first: an identity without its key must never load.
	if err := s.writeVerified(keyFile, []byte(base64.StdEncoding.EncodeToString(privateKey)), 0o600); err != nil {
		return ports.NodeIdentity{}, err
	}
	encoded, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return ports.NodeIdentity{}, err
	}
	if err := s.writeVerified(identityFile, append(encoded, '\n'), 0o600); err != nil {
		return ports.NodeIdentity{}, err
	}
	return identity, nil
}

// Rotate replaces the keypair, keeping the node id and recording the rotation.
func (s *Store) Rotate(_ context.Context) (ports.NodeIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.identityLocked()
	if err != nil {
		return ports.NodeIdentity{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return ports.NodeIdentity{}, err
	}
	sum := sha256.Sum256(publicKey)
	previous.PublicKey = base64.StdEncoding.EncodeToString(publicKey)
	previous.Fingerprint = hex.EncodeToString(sum[:])
	previous.RotatedAt = time.Now().UTC()
	if err := s.writeVerified(keyFile, []byte(base64.StdEncoding.EncodeToString(privateKey)), 0o600); err != nil {
		return ports.NodeIdentity{}, err
	}
	encoded, err := json.MarshalIndent(previous, "", "  ")
	if err != nil {
		return ports.NodeIdentity{}, err
	}
	if err := s.writeVerified(identityFile, append(encoded, '\n'), 0o600); err != nil {
		return ports.NodeIdentity{}, err
	}
	return previous, nil
}

func (s *Store) readPeers() ([]ports.RegisteredNode, error) {
	raw, err := os.ReadFile(s.path(peersFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var nodes []ports.RegisteredNode
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, fmt.Errorf("parse node registry: %w", err)
	}
	return nodes, nil
}

// Register adds or replaces a peer registration.
func (s *Store) Register(_ context.Context, node ports.RegisteredNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(node.NodeID) == "" || strings.TrimSpace(node.Fingerprint) == "" {
		return errors.New("node id and fingerprint are required")
	}
	nodes, err := s.readPeers()
	if err != nil {
		return err
	}
	if node.RegisteredAt.IsZero() {
		node.RegisteredAt = time.Now().UTC()
	}
	replaced := false
	for index, existing := range nodes {
		if existing.NodeID == node.NodeID {
			nodes[index] = node
			replaced = true
		}
	}
	if !replaced {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeID < nodes[j].NodeID })
	encoded, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}
	return s.writeVerified(peersFile, append(encoded, '\n'), 0o600)
}

// Revoke marks a peer revoked; it stays listed for audit and fails closed.
func (s *Store) Revoke(_ context.Context, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.readPeers()
	if err != nil {
		return err
	}
	found := false
	for index, existing := range nodes {
		if existing.NodeID == nodeID {
			nodes[index].RevokedAt = time.Now().UTC()
			found = true
		}
	}
	if !found {
		return ports.ErrNodeUnknown
	}
	encoded, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}
	return s.writeVerified(peersFile, append(encoded, '\n'), 0o600)
}

// Authorize fails closed for unknown, revoked and fingerprint-mismatched peers.
func (s *Store) Authorize(_ context.Context, nodeID, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.readPeers()
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if node.NodeID != nodeID {
			continue
		}
		if !node.RevokedAt.IsZero() {
			return ports.ErrNodeRevoked
		}
		if node.Fingerprint != fingerprint {
			return fmt.Errorf("node %s presented a fingerprint that is not registered", nodeID)
		}
		return nil
	}
	return ports.ErrNodeUnknown
}

// List returns every registered peer.
func (s *Store) List(_ context.Context) ([]ports.RegisteredNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.readPeers()
	if err != nil {
		return nil, err
	}
	if nodes == nil {
		nodes = []ports.RegisteredNode{}
	}
	return nodes, nil
}

// Ledger is the durable idempotency ledger.
type Ledger struct{ store *Store }

// Ledger returns the task ledger sharing this store's directory.
func (s *Store) Ledger() *Ledger { return &Ledger{store: s} }

func (l *Ledger) read() ([]ports.TaskRecord, error) {
	raw, err := os.ReadFile(l.store.path(ledgerFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var records []ports.TaskRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("parse task ledger: %w", err)
	}
	return records, nil
}

func (l *Ledger) write(records []ports.TaskRecord) error {
	sort.Slice(records, func(i, j int) bool { return records[i].RequestID < records[j].RequestID })
	encoded, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return l.store.writeVerified(ledgerFile, append(encoded, '\n'), 0o600)
}

// Begin claims a request id. The second return value reports "already existed"
// so callers can return the stored result instead of executing again.
func (l *Ledger) Begin(_ context.Context, record ports.TaskRecord) (ports.TaskRecord, bool, error) {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	records, err := l.read()
	if err != nil {
		return ports.TaskRecord{}, false, err
	}
	for _, existing := range records {
		if existing.RequestID != record.RequestID {
			continue
		}
		if existing.InputHash != record.InputHash || existing.Operation != record.Operation {
			return ports.TaskRecord{}, false, ports.ErrTaskConflict
		}
		return existing, true, nil
	}
	if record.State == "" {
		record.State = ports.TaskPending
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	record.UpdatedAt = record.CreatedAt
	if err := l.write(append(records, record)); err != nil {
		return ports.TaskRecord{}, false, err
	}
	return record, false, nil
}

// Complete records the terminal state of a task exactly once.
func (l *Ledger) Complete(_ context.Context, requestID, result, errText string) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	records, err := l.read()
	if err != nil {
		return err
	}
	found := false
	for index, record := range records {
		if record.RequestID != requestID {
			continue
		}
		found = true
		if record.State != ports.TaskPending {
			// Already terminal: completing twice is a no-op, never an overwrite.
			return nil
		}
		records[index].Result = result
		records[index].Error = errText
		if errText != "" {
			records[index].State = ports.TaskFailed
		} else {
			records[index].State = ports.TaskCompleted
		}
		records[index].UpdatedAt = time.Now().UTC()
	}
	if !found {
		return errors.New("task is not in the ledger")
	}
	return l.write(records)
}

// Get returns one task.
func (l *Ledger) Get(_ context.Context, requestID string) (ports.TaskRecord, error) {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	records, err := l.read()
	if err != nil {
		return ports.TaskRecord{}, err
	}
	for _, record := range records {
		if record.RequestID == requestID {
			return record, nil
		}
	}
	return ports.TaskRecord{}, errors.New("task not found")
}

// List returns every task record.
func (l *Ledger) List(_ context.Context) ([]ports.TaskRecord, error) {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	records, err := l.read()
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []ports.TaskRecord{}
	}
	return records, nil
}

var _ ports.NodeStore = (*Store)(nil)
var _ ports.TaskLedger = (*Ledger)(nil)
