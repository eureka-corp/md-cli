package mdshare

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Record is a share this CLI created, remembered so it can be updated in place.
// ExpiresAt is nil for a share that never expires.
type Record struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (r Record) expired(now time.Time) bool {
	return r.ExpiresAt != nil && r.ExpiresAt.Before(now)
}

// State is the file of remembered shares. Fallback, if set, is read when a key
// is not found, so shares made with `orion md share` can still be updated.
type State struct {
	Path     string
	Fallback string
}

// StateKey identifies what was shared: the absolute paths, order-independent.
func StateKey(args []string, stdinName string) (string, error) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-" {
			parts = append(parts, "stdin:"+stdinName)
			continue
		}
		abs, err := filepath.Abs(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, abs)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n"), nil
}

func load(path string) (map[string]Record, error) {
	state := map[string]Record{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (s State) save(state map[string]Record) error {
	now := time.Now()
	for k, r := range state {
		if r.expired(now) {
			delete(state, k)
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}

// Lookup returns the unexpired share remembered for key, if any.
func (s State) Lookup(key string) (Record, bool) {
	for _, p := range []string{s.Path, s.Fallback} {
		if p == "" {
			continue
		}
		state, err := load(p)
		if err != nil {
			continue
		}
		if rec, ok := state[key]; ok && !rec.expired(time.Now()) {
			return rec, true
		}
	}
	return Record{}, false
}

// Remember stores the share for key, replacing any previous one.
func (s State) Remember(key string, share *Share) error {
	state, err := load(s.Path)
	if err != nil {
		// A corrupt state file only costs the remembered links.
		state = map[string]Record{}
	}
	state[key] = Record{ID: share.ID, URL: share.URL, ExpiresAt: share.ExpiresAt}
	return s.save(state)
}

// SetExpiry records a new expiry for every entry pointing at id.
func (s State) SetExpiry(id string, expiresAt *time.Time) error {
	return s.edit(func(state map[string]Record) {
		for k, r := range state {
			if r.ID == id {
				r.ExpiresAt = expiresAt
				state[k] = r
			}
		}
	})
}

// Forget drops every record pointing at the share id.
func (s State) Forget(id string) error {
	return s.edit(func(state map[string]Record) {
		for k, r := range state {
			if r.ID == id {
				delete(state, k)
			}
		}
	})
}

func (s State) edit(change func(map[string]Record)) error {
	state, err := load(s.Path)
	if err != nil {
		return err
	}
	change(state)
	return s.save(state)
}
