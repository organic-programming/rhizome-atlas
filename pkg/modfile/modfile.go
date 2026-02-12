// Package modfile parses and writes holon.mod and holon.sum files.
// The format is deliberately modeled on Go modules (go.mod / go.sum).
package modfile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModFile represents a parsed holon.mod file.
type ModFile struct {
	HolonPath string
	Require   []Require
	Replace   []Replace
}

// Require is a single dependency declaration.
type Require struct {
	Path    string
	Version string
}

// Replace is a local path override for a dependency.
type Replace struct {
	Old       string // remote path
	LocalPath string // local directory (relative to holon.mod)
}

// Parse reads and parses a holon.mod file.
func Parse(path string) (*ModFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mod := &ModFile{}
	scanner := bufio.NewScanner(f)
	var inBlock string // "require" or "replace"

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Block boundaries
		if line == ")" {
			inBlock = ""
			continue
		}
		if line == "require (" {
			inBlock = "require"
			continue
		}
		if line == "replace (" {
			inBlock = "replace"
			continue
		}

		// Holon directive
		if strings.HasPrefix(line, "holon ") {
			mod.HolonPath = strings.TrimPrefix(line, "holon ")
			continue
		}

		// Inside a block
		switch inBlock {
		case "require":
			parts := strings.Fields(line)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid require line: %q", line)
			}
			mod.Require = append(mod.Require, Require{Path: parts[0], Version: parts[1]})

		case "replace":
			// Format: <old> => <local>
			parts := strings.SplitN(line, " => ", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid replace line: %q", line)
			}
			mod.Replace = append(mod.Replace, Replace{
				Old:       strings.TrimSpace(parts[0]),
				LocalPath: strings.TrimSpace(parts[1]),
			})
		}
	}

	return mod, scanner.Err()
}

// Write serializes a ModFile to disk.
func (m *ModFile) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "holon %s\n", m.HolonPath)

	if len(m.Require) > 0 {
		fmt.Fprintln(f)
		fmt.Fprintln(f, "require (")
		for _, r := range m.Require {
			fmt.Fprintf(f, "    %s %s\n", r.Path, r.Version)
		}
		fmt.Fprintln(f, ")")
	}

	if len(m.Replace) > 0 {
		fmt.Fprintln(f)
		fmt.Fprintln(f, "replace (")
		for _, r := range m.Replace {
			fmt.Fprintf(f, "    %s => %s\n", r.Old, r.LocalPath)
		}
		fmt.Fprintln(f, ")")
	}

	return nil
}

// AddRequire adds or updates a dependency. Returns true if it was added
// (false if updated).
func (m *ModFile) AddRequire(path, version string) bool {
	for i, r := range m.Require {
		if r.Path == path {
			m.Require[i].Version = version
			return false
		}
	}
	m.Require = append(m.Require, Require{Path: path, Version: version})
	return true
}

// RemoveRequire removes a dependency by path. Returns true if found.
func (m *ModFile) RemoveRequire(path string) bool {
	for i, r := range m.Require {
		if r.Path == path {
			m.Require = append(m.Require[:i], m.Require[i+1:]...)
			return true
		}
	}
	return false
}

// ResolvedPath returns the local path for a dependency if a replace
// directive exists, otherwise empty string.
func (m *ModFile) ResolvedPath(depPath string) string {
	for _, r := range m.Replace {
		if r.Old == depPath {
			return r.LocalPath
		}
	}
	return ""
}

// --- holon.sum ---

// SumEntry represents one line in holon.sum.
type SumEntry struct {
	Path    string // e.g. "github.com/org/dep"
	Version string // e.g. "v1.2.0" or "v1.2.0/HOLON.md"
	Hash    string // e.g. "h1:abc123..."
}

// SumFile represents a parsed holon.sum.
type SumFile struct {
	Entries []SumEntry
}

// ParseSum reads and parses a holon.sum file.
func ParseSum(path string) (*SumFile, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SumFile{}, nil
		}
		return nil, err
	}
	defer f.Close()

	sum := &SumFile{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid holon.sum line: %q", line)
		}
		sum.Entries = append(sum.Entries, SumEntry{
			Path:    parts[0],
			Version: parts[1],
			Hash:    parts[2],
		})
	}
	return sum, scanner.Err()
}

// Write serializes a SumFile to disk.
func (s *SumFile) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Sort entries for deterministic output
	sort.Slice(s.Entries, func(i, j int) bool {
		if s.Entries[i].Path != s.Entries[j].Path {
			return s.Entries[i].Path < s.Entries[j].Path
		}
		return s.Entries[i].Version < s.Entries[j].Version
	})

	for _, e := range s.Entries {
		fmt.Fprintf(f, "%s %s %s\n", e.Path, e.Version, e.Hash)
	}
	return nil
}

// Set adds or updates an entry. If an entry with the same path+version
// exists, it is replaced.
func (s *SumFile) Set(path, version, hash string) {
	for i, e := range s.Entries {
		if e.Path == path && e.Version == version {
			s.Entries[i].Hash = hash
			return
		}
	}
	s.Entries = append(s.Entries, SumEntry{Path: path, Version: version, Hash: hash})
}

// Lookup returns the hash for a given path+version, or empty string.
func (s *SumFile) Lookup(path, version string) string {
	for _, e := range s.Entries {
		if e.Path == path && e.Version == version {
			return e.Hash
		}
	}
	return ""
}
