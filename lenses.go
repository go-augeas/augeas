// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"fmt"
	"strings"
)

// This file ships a starter set of built-in lenses. Each round-trips a
// canonical, newline-terminated text form to the Augeas tree and back. The
// canonical forms are documented in the README; they intentionally cover only a
// fraction of the ~200 lenses in upstream Augeas (see the deferred list).

func init() {
	Register("Hosts", hostsLens{})
	Register("Fstab", fstabLens{})
	sv := shellvarsLens{}
	Register("Shellvars", sv)
	Register("Simplevars", sv)
	ini := iniLens{}
	Register("Ini", ini)
	Register("Keyvalue", ini)
}

// nodeVal returns the node's value, or "" when the node is nil or valueless.
func nodeVal(n *Node) string {
	if n == nil || n.Value == nil {
		return ""
	}
	return *n.Value
}

// splitLines splits text on '\n', dropping the single empty element produced by
// a trailing newline. Empty text yields no lines.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// isComment reports whether a trimmed line is a comment and returns the comment
// body (text after the leading marker, trimmed).
func isComment(trimmed string, markers string) (string, bool) {
	if trimmed == "" {
		return "", false
	}
	if strings.ContainsRune(markers, rune(trimmed[0])) {
		return strings.TrimSpace(trimmed[1:]), true
	}
	return "", false
}

// addChild appends a child with the given label and value to parent.
func addChild(parent *Node, label, value string) {
	c := newNode(label)
	c.SetValue(value)
	parent.appendChild(c)
}

// ---- Hosts (/etc/hosts) --------------------------------------------------

type hostsLens struct{}

func (hostsLens) Parse(text string) (*Node, error) {
	root := &Node{}
	seq := 0
	for _, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if body, ok := isComment(trimmed, "#"); ok {
			addChild(root, "#comment", body)
			continue
		}
		data, comment := line, ""
		if i := strings.IndexByte(line, '#'); i >= 0 {
			data, comment = line[:i], strings.TrimSpace(line[i+1:])
		}
		fields := strings.Fields(data)
		if len(fields) < 2 {
			return nil, fmt.Errorf("hosts: line %q needs an address and canonical name", line)
		}
		seq++
		entry := newNode(fmt.Sprintf("%d", seq))
		addChild(entry, "ipaddr", fields[0])
		addChild(entry, "canonical", fields[1])
		for _, alias := range fields[2:] {
			addChild(entry, "alias", alias)
		}
		if comment != "" {
			addChild(entry, "#comment", comment)
		}
		root.appendChild(entry)
	}
	return root, nil
}

func (hostsLens) Build(root *Node) (string, error) {
	var sb strings.Builder
	for _, c := range root.Children {
		if c.Label == "#comment" {
			sb.WriteString("# " + nodeVal(c) + "\n")
			continue
		}
		ip := c.firstChild("ipaddr")
		canonical := c.firstChild("canonical")
		if ip == nil || canonical == nil {
			return "", fmt.Errorf("hosts: entry %q missing ipaddr or canonical", c.Label)
		}
		parts := []string{nodeVal(ip), nodeVal(canonical)}
		for _, a := range c.Children {
			if a.Label == "alias" {
				parts = append(parts, nodeVal(a))
			}
		}
		line := strings.Join(parts, " ")
		if cc := c.firstChild("#comment"); cc != nil {
			line += " # " + nodeVal(cc)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String(), nil
}

// ---- Fstab (/etc/fstab) --------------------------------------------------

type fstabLens struct{}

func (fstabLens) Parse(text string) (*Node, error) {
	root := &Node{}
	seq := 0
	for _, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if body, ok := isComment(trimmed, "#"); ok {
			addChild(root, "#comment", body)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return nil, fmt.Errorf("fstab: line %q needs at least 4 fields", line)
		}
		if len(fields) > 6 {
			return nil, fmt.Errorf("fstab: line %q has more than 6 fields", line)
		}
		seq++
		entry := newNode(fmt.Sprintf("%d", seq))
		addChild(entry, "spec", fields[0])
		addChild(entry, "file", fields[1])
		addChild(entry, "vfstype", fields[2])
		for _, opt := range strings.Split(fields[3], ",") {
			addChild(entry, "opt", opt)
		}
		if len(fields) >= 5 {
			addChild(entry, "dump", fields[4])
		}
		if len(fields) >= 6 {
			addChild(entry, "passno", fields[5])
		}
		root.appendChild(entry)
	}
	return root, nil
}

func (fstabLens) Build(root *Node) (string, error) {
	var sb strings.Builder
	for _, c := range root.Children {
		if c.Label == "#comment" {
			sb.WriteString("# " + nodeVal(c) + "\n")
			continue
		}
		spec := c.firstChild("spec")
		file := c.firstChild("file")
		vfstype := c.firstChild("vfstype")
		if spec == nil || file == nil || vfstype == nil {
			return "", fmt.Errorf("fstab: entry %q missing spec, file or vfstype", c.Label)
		}
		var opts []string
		for _, o := range c.Children {
			if o.Label == "opt" {
				opts = append(opts, nodeVal(o))
			}
		}
		if len(opts) == 0 {
			return "", fmt.Errorf("fstab: entry %q has no mount options", c.Label)
		}
		parts := []string{nodeVal(spec), nodeVal(file), nodeVal(vfstype), strings.Join(opts, ",")}
		if d := c.firstChild("dump"); d != nil {
			parts = append(parts, nodeVal(d))
		}
		if p := c.firstChild("passno"); p != nil {
			parts = append(parts, nodeVal(p))
		}
		sb.WriteString(strings.Join(parts, " ") + "\n")
	}
	return sb.String(), nil
}

// ---- Shellvars / Simplevars (KEY=value) ----------------------------------

type shellvarsLens struct{}

func (shellvarsLens) Parse(text string) (*Node, error) {
	root := &Node{}
	for _, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if body, ok := isComment(trimmed, "#"); ok {
			addChild(root, "#comment", body)
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			return nil, fmt.Errorf("shellvars: line %q is not KEY=value", line)
		}
		addChild(root, line[:i], line[i+1:])
	}
	return root, nil
}

func (shellvarsLens) Build(root *Node) (string, error) {
	var sb strings.Builder
	for _, c := range root.Children {
		if c.Label == "#comment" {
			sb.WriteString("# " + nodeVal(c) + "\n")
			continue
		}
		sb.WriteString(c.Label + "=" + nodeVal(c) + "\n")
	}
	return sb.String(), nil
}

// ---- Ini / Keyvalue ------------------------------------------------------

type iniLens struct{}

func (iniLens) Parse(text string) (*Node, error) {
	root := &Node{}
	cur := root
	for _, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if body, ok := isComment(trimmed, "#;"); ok {
			addChild(cur, "#comment", body)
			continue
		}
		if trimmed[0] == '[' {
			if trimmed[len(trimmed)-1] != ']' {
				return nil, fmt.Errorf("ini: bad section header %q", line)
			}
			name := trimmed[1 : len(trimmed)-1]
			if name == "" {
				return nil, fmt.Errorf("ini: empty section name in %q", line)
			}
			sec := newNode(name)
			root.appendChild(sec)
			cur = sec
			continue
		}
		i := strings.IndexByte(trimmed, '=')
		if i < 0 {
			return nil, fmt.Errorf("ini: line %q is not key=value", line)
		}
		key := strings.TrimSpace(trimmed[:i])
		if key == "" {
			return nil, fmt.Errorf("ini: empty key in %q", line)
		}
		addChild(cur, key, strings.TrimSpace(trimmed[i+1:]))
	}
	return root, nil
}

func (iniLens) Build(root *Node) (string, error) {
	var sb strings.Builder
	for _, c := range root.Children {
		switch {
		case c.Label == "#comment":
			sb.WriteString("# " + nodeVal(c) + "\n")
		case c.Value == nil:
			sb.WriteString("[" + c.Label + "]\n")
			for _, kc := range c.Children {
				if kc.Label == "#comment" {
					sb.WriteString("# " + nodeVal(kc) + "\n")
					continue
				}
				sb.WriteString(kc.Label + " = " + nodeVal(kc) + "\n")
			}
		default:
			sb.WriteString(c.Label + " = " + nodeVal(c) + "\n")
		}
	}
	return sb.String(), nil
}
