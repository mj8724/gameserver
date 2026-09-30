// Package auditlog is an append-only audit trail (M6.2). Records are appended
// as JSON lines, flushed to disk, and never rewritten in place.
package auditlog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mj8724/gameserver/internal/ports"
)

const fileName = "audit.jsonl"

// Log is a file-backed append-only audit trail.
type Log struct {
	path string
	mu   sync.Mutex
}

// New opens (or creates) the audit trail at <root>/audit.jsonl with 0600.
func New(root string) (*Log, error) {
	if strings.TrimSpace(root) == "" || strings.ContainsRune(root, '\x00') {
		return nil, errors.New("audit root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return &Log{path: filepath.Join(absolute, fileName)}, nil
}

// Append writes one record and flushes it before returning.
func (l *Log) Append(_ context.Context, entry ports.AuditEntry) error {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

// Recent returns the newest records, oldest first.
func (l *Log) Recent(_ context.Context, limit int) ([]ports.AuditEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ports.AuditEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()
	var entries []ports.AuditEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry ports.AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// A corrupt line fails closed: the trail is not silently skipped.
			return nil, errors.New("audit trail contains an unreadable record")
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	if entries == nil {
		entries = []ports.AuditEntry{}
	}
	return entries, nil
}

var _ ports.AuditLog = (*Log)(nil)
