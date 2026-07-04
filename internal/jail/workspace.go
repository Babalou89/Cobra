// Package jail confines every agent write to one directory. The workspace
// is the agent's entire world: all tool paths resolve through it, and any
// path that escapes it is rejected before touching the filesystem.
package jail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is the one jailed directory the agent may touch.
type Workspace struct {
	Root string
}

// New creates (or reuses) the jail rooted at root.
func New(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	// Resolve symlinks so prefix checks cannot be bypassed via a link.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Workspace{Root: resolved}, nil
}

// Resolve maps a path (relative to the jail, or absolute) to an absolute
// path guaranteed to be inside the jail. Anything else is an error.
func (w *Workspace) Resolve(p string) (string, error) {
	orig := p
	if !filepath.IsAbs(p) {
		p = filepath.Join(w.Root, p)
	}
	p = filepath.Clean(p)
	rel, err := filepath.Rel(w.Root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the jail (%s)", orig, w.Root)
	}
	return p, nil
}

// WriteFile writes inside the jail, creating parent dirs as needed.
func (w *Workspace) WriteFile(p string, data []byte) (string, error) {
	abs, err := w.Resolve(p)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	return abs, os.WriteFile(abs, data, 0o644)
}

// ReadFile reads a file inside the jail.
func (w *Workspace) ReadFile(p string) ([]byte, error) {
	abs, err := w.Resolve(p)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// Teardown removes the jail directory entirely.
func (w *Workspace) Teardown() error {
	return os.RemoveAll(w.Root)
}
