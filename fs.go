// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// FileSystem is the seam through which Load and Save reach files. The default
// implementation ([New] installs it) is the real OS; tests inject an in-memory
// implementation so the engine can be exercised without touching disk.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Glob(pattern string) ([]string, error)
}

// osFS is the default FileSystem backed by the os package.
type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

func (osFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (osFS) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }

// Load reads every file matching the glob pattern, parses it with lens and
// stores the resulting tree under mount+"/"+basename. A read or parse failure
// for one file is recorded under /augeas/files/<basename>/error and the
// remaining files are still processed; only a failing glob is returned as an
// error.
func (a *Augeas) Load(lens Lens, pattern, mount string) error {
	files, err := a.fs.Glob(pattern)
	if err != nil {
		return a.fail(err)
	}
	for _, f := range files {
		base := path.Base(filepath.ToSlash(f))
		data, err := a.fs.ReadFile(f)
		if err != nil {
			a.recordError(base, err.Error())
			continue
		}
		parsed, err := lens.Parse(string(data))
		if err != nil {
			a.recordError(base, err.Error())
			continue
		}
		dst, err := a.createFrom(a.root, trimLeadingSlash(mount)+"/"+base)
		if err != nil {
			a.recordError(base, err.Error())
			continue
		}
		dst.Children = nil
		for _, c := range parsed.Children {
			dst.appendChild(c)
		}
	}
	return nil
}

// Save serialises the single subtree at mount with lens and writes it to
// filename through the filesystem seam.
func (a *Augeas) Save(lens Lens, mount, filename string) error {
	nodes, err := a.eval(mount)
	if err != nil {
		return a.fail(err)
	}
	if len(nodes) != 1 {
		return a.fail(fmt.Errorf("save mount %q matches %d nodes", mount, len(nodes)))
	}
	text, err := lens.Build(nodes[0])
	if err != nil {
		return a.fail(err)
	}
	if err := a.fs.WriteFile(filename, []byte(text), 0o644); err != nil {
		return a.fail(err)
	}
	return nil
}

// recordError stores msg under /augeas/files/<base>/error so load failures are
// visible in the tree, matching Augeas' /augeas//error metadata.
func (a *Augeas) recordError(base, msg string) {
	n, err := a.createFrom(a.root, "augeas/files/"+base+"/error")
	if err != nil {
		return
	}
	n.SetValue(msg)
}

// trimLeadingSlash removes a single leading slash from p.
func trimLeadingSlash(p string) string {
	if len(p) > 0 && p[0] == '/' {
		return p[1:]
	}
	return p
}
