package config

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// ErrStoreClosed is returned when a workspace has already stopped admitting
// configuration reads or writes. A closed store is never reopened; the
// workspace lifecycle owns that boundary.
var ErrStoreClosed = errors.New("配置存储已关闭")

// Store serializes the complete config read/normalize/mutate/save transaction
// for one immutable workspace. It deliberately knows nothing about App,
// Wails, or envsetup; callers decide what to do with a committed snapshot
// after this lock has been released.
//
// Store does not cache Config values. Each Snapshot/Update observes the file
// through the existing LoadOrCreate compatibility path, so external edits
// retain the historical read semantics while all in-process writers share one
// lock.
type Store struct {
	path     string
	dataRoot string

	mu       sync.Mutex
	closed   atomic.Bool
	revision uint64

	// saveFn is intentionally package-private so config tests can inject a
	// failed save without weakening the production Save implementation.
	saveFn func(string, *Config) error
}

// NewStore binds a store to one config path and data root. The paths are
// normalized once and never follow a later App dataDir change.
func NewStore(path, dataRoot string) *Store {
	if path == "" && dataRoot != "" {
		path = filepath.Join(dataRoot, "config.json")
	}
	if path != "" {
		path = filepath.Clean(path)
	}
	if dataRoot != "" {
		dataRoot = filepath.Clean(dataRoot)
	}
	return &Store{path: path, dataRoot: dataRoot}
}

// Path returns the immutable config path bound at construction time.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// DataRoot returns the immutable workspace root bound at construction time.
func (s *Store) DataRoot() string {
	if s == nil {
		return ""
	}
	return s.dataRoot
}

// IsClosed reports whether the workspace lifecycle has closed this store.
func (s *Store) IsClosed() bool {
	if s == nil {
		return true
	}
	return s.closed.Load()
}

// Close permanently closes the store. It is idempotent and does not remove
// or otherwise mutate the config file.
func (s *Store) Close() {
	if s == nil {
		return
	}
	s.closed.Store(true)
}

// Snapshot loads and normalizes the latest config under the store lock and
// returns a deep-enough copy that callers may freely mutate.
func (s *Store) Snapshot() (*Config, error) {
	cfg, _, err := s.SnapshotWithRevision()
	return cfg, err
}

// SnapshotWithRevision is Snapshot plus the monotonically increasing commit
// observation used by envsetup to reject an older post-commit publication.
func (s *Store) SnapshotWithRevision() (*Config, uint64, error) {
	if s == nil {
		return nil, 0, ErrStoreClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return nil, s.revision, ErrStoreClosed
	}
	cfg, err := s.loadOrCreateLocked()
	if err != nil {
		return nil, s.revision, err
	}
	s.revision++
	return cloneConfig(cfg), s.revision, nil
}

// Update executes the complete read/normalize/mutate/save transaction and
// returns the committed snapshot. The mutate callback runs while the store is
// locked and must not perform blocking I/O, call this Store again, or emit
// events.
func (s *Store) Update(mutate func(*Config) error) (*Config, error) {
	cfg, _, err := s.UpdateWithRevision(mutate)
	return cfg, err
}

// UpdateWithRevision is Update plus a commit observation for consumers that
// keep a derived, in-memory snapshot. The revision is advanced only after a
// successful Save, so failed mutations and failed saves publish nothing.
func (s *Store) UpdateWithRevision(mutate func(*Config) error) (*Config, uint64, error) {
	if s == nil {
		return nil, 0, ErrStoreClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return nil, s.revision, ErrStoreClosed
	}
	cfg, err := s.loadOrCreateLocked()
	if err != nil {
		return nil, s.revision, err
	}
	if mutate != nil {
		if err := mutate(cfg); err != nil {
			return nil, s.revision, err
		}
	}
	if s.closed.Load() {
		return nil, s.revision, ErrStoreClosed
	}
	if err := s.saveLocked(cfg); err != nil {
		return nil, s.revision, err
	}
	s.revision++
	return cloneConfig(cfg), s.revision, nil
}

// EnsureFirstInstallChangelogSeed preserves the first-install ordering rule
// while sharing the same transaction lock as all subsequent config access.
func (s *Store) EnsureFirstInstallChangelogSeed(currentVersion string) (bool, error) {
	if s == nil {
		return false, ErrStoreClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return false, ErrStoreClosed
	}
	seeded, err := ensureFirstInstallChangelogSeed(s.path, s.dataRoot, currentVersion, s.saveAtLocked)
	if err == nil && seeded {
		s.revision++
	}
	return seeded, err
}

func (s *Store) saveLocked(cfg *Config) error {
	return s.saveAtLocked(s.path, cfg)
}

func (s *Store) saveAtLocked(path string, cfg *Config) error {
	if s.saveFn != nil {
		return s.saveFn(path, cfg)
	}
	return Save(path, cfg)
}

func (s *Store) loadOrCreateLocked() (*Config, error) {
	return loadOrCreate(s.path, s.dataRoot, s.saveAtLocked)
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	clone.FFmpegDetectedEncoders = append([]string(nil), cfg.FFmpegDetectedEncoders...)
	if cfg.ClipActionSettings != nil {
		settings := *cfg.ClipActionSettings
		clone.ClipActionSettings = &settings
	}
	return &clone
}

// Clone returns an independent Config value suitable for passing across a
// package boundary. It copies the mutable encoder slice and clip-action
// pointer in addition to the value fields.
func Clone(cfg *Config) *Config {
	return cloneConfig(cfg)
}
