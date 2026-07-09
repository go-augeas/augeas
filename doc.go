// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

// Package augeas is a pure-Go (no cgo, stdlib-only) implementation of the core
// of Augeas, the configuration-editing library from the Puppet ecosystem.
//
// Augeas models configuration files as an ordered tree and exposes an
// XPath-like path language to read and edit that tree; lenses translate the
// tree back and forth to the concrete file syntax. This package provides:
//
//   - a tree model ([Node]) of ordered, labelled nodes with optional values
//     (siblings that share a label are addressed with 1-based positional
//     indices, exactly like Augeas);
//   - an Augeas-subset path evaluator (see [Augeas.Match]) backing the whole
//     read/write API;
//   - the core editing API on [Augeas]: Get, Exists, Set, SetMultiple, Insert,
//     Remove, Move, Match, Label, DefineVariable, DefineNode and an Error
//     surface, plus a minimal Span;
//   - a [Lens] framework (Parse/Build round-tripping text and tree) with the
//     in-memory [Augeas.TextStore] and [Augeas.TextRetrieve] helpers;
//   - a starter set of built-in lenses (Hosts, Fstab, Shellvars/Simplevars and
//     Ini/Keyvalue), registered by name;
//   - file [Augeas.Load] and [Augeas.Save] through a lens over an injectable
//     [FileSystem] seam (the default seam is the real OS).
//
// Supported path constructs, the shipped lenses and the explicit list of what
// is deferred are documented in the README. This package has no cgo and no
// third-party dependencies.
package augeas
