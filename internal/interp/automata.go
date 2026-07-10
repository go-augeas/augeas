// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"regexp/syntax"
	"sort"
)

// This file implements the finite-automata machinery Augeas needs for regexp
// subtraction (the `-` operator). Upstream computes r1 - r2 as r1 ∩ ¬r2 over a
// deterministic automaton and converts the result back to a regexp string
// (fa_minus + fa_as_regexp). We do the same in pure Go: parse each operand with
// the standard library's regexp/syntax, build a byte-level NFA over a shared
// symbol alphabet, determinise, complement, intersect, and finally emit a
// regexp via state elimination. The emitted pattern is matched by the same RE2
// engine as every other lens regexp, so it is self-consistent with get.

// alphabet partitions [0,255] into disjoint intervals ("symbols") chosen so
// that every character-class boundary of both operands falls on an interval
// edge. Each symbol can then be treated atomically.
type alphabet struct {
	cuts []int // sorted boundaries; intervals are [cuts[i], cuts[i+1]-1]
}

func newAlphabet(res ...*syntax.Regexp) *alphabet {
	set := map[int]struct{}{0: {}, 256: {}}
	for _, re := range res {
		collectBounds(re, set)
	}
	cuts := make([]int, 0, len(set))
	for c := range set {
		if c >= 0 && c <= 256 {
			cuts = append(cuts, c)
		}
	}
	sort.Ints(cuts)
	return &alphabet{cuts: cuts}
}

func (a *alphabet) nsym() int { return len(a.cuts) - 1 }

// symsForRange returns the symbol indices fully inside [lo,hi] (clamped to
// 0..255).
func (a *alphabet) symsForRange(lo, hi int) []int {
	if hi > 255 {
		hi = 255
	}
	var out []int
	for s := 0; s < a.nsym(); s++ {
		if a.cuts[s] >= lo && a.cuts[s+1]-1 <= hi {
			out = append(out, s)
		}
	}
	return out
}

// interval returns the byte range covered by symbol s.
func (a *alphabet) interval(s int) (int, int) { return a.cuts[s], a.cuts[s+1] - 1 }

func addBound(set map[int]struct{}, lo, hi int) {
	set[lo] = struct{}{}
	set[hi+1] = struct{}{}
}

func collectBounds(re *syntax.Regexp, set map[int]struct{}) {
	switch re.Op {
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			if r < 256 {
				addBound(set, int(r), int(r))
			} else {
				for _, b := range []byte(string(r)) {
					addBound(set, int(b), int(b))
				}
			}
		}
	case syntax.OpCharClass:
		for i := 0; i+1 < len(re.Rune); i += 2 {
			addBound(set, int(re.Rune[i]), int(re.Rune[i+1]))
		}
	case syntax.OpAnyCharNotNL:
		addBound(set, '\n', '\n')
	}
	for _, sub := range re.Sub {
		collectBounds(sub, set)
	}
}

// nfa is a Thompson automaton over symbol indices; symbol -1 is epsilon.
type nfa struct {
	trans  []map[int][]int // state -> symbol -> states
	start  int
	accept int
}

func newNFA() *nfa { return &nfa{} }

func (n *nfa) newState() int {
	n.trans = append(n.trans, map[int][]int{})
	return len(n.trans) - 1
}

func (n *nfa) addEdge(from, sym, to int) {
	n.trans[from][sym] = append(n.trans[from][sym], to)
}

// frag is an NFA fragment with a single start and single accept state.
type frag struct{ start, accept int }

func buildNFA(re *syntax.Regexp, ab *alphabet) *nfa {
	n := newNFA()
	f := n.build(re, ab)
	n.start = f.start
	n.accept = f.accept
	return n
}

// epsilonFrag returns a fragment that matches the empty string.
func (n *nfa) epsilonFrag() frag {
	s := n.newState()
	a := n.newState()
	n.addEdge(s, -1, a)
	return frag{s, a}
}

func (n *nfa) build(re *syntax.Regexp, ab *alphabet) frag {
	switch re.Op {
	case syntax.OpLiteral:
		return n.literalFrag(re.Rune, ab)
	case syntax.OpCharClass:
		var syms []int
		for i := 0; i+1 < len(re.Rune); i += 2 {
			syms = append(syms, ab.symsForRange(int(re.Rune[i]), int(re.Rune[i+1]))...)
		}
		return n.symsFrag(syms)
	case syntax.OpAnyChar:
		return n.symsFrag(ab.symsForRange(0, 255))
	case syntax.OpAnyCharNotNL:
		var syms []int
		syms = append(syms, ab.symsForRange(0, '\n'-1)...)
		syms = append(syms, ab.symsForRange('\n'+1, 255)...)
		return n.symsFrag(syms)
	case syntax.OpCapture:
		return n.build(re.Sub[0], ab)
	case syntax.OpConcat:
		f := n.build(re.Sub[0], ab)
		for _, sub := range re.Sub[1:] {
			g := n.build(sub, ab)
			n.addEdge(f.accept, -1, g.start)
			f.accept = g.accept
		}
		return f
	case syntax.OpAlternate:
		s := n.newState()
		a := n.newState()
		for _, sub := range re.Sub {
			g := n.build(sub, ab)
			n.addEdge(s, -1, g.start)
			n.addEdge(g.accept, -1, a)
		}
		return frag{s, a}
	case syntax.OpStar:
		g := n.build(re.Sub[0], ab)
		s := n.newState()
		a := n.newState()
		n.addEdge(s, -1, g.start)
		n.addEdge(s, -1, a)
		n.addEdge(g.accept, -1, g.start)
		n.addEdge(g.accept, -1, a)
		return frag{s, a}
	case syntax.OpPlus:
		g := n.build(re.Sub[0], ab)
		a := n.newState()
		n.addEdge(g.accept, -1, g.start)
		n.addEdge(g.accept, -1, a)
		return frag{g.start, a}
	case syntax.OpQuest:
		g := n.build(re.Sub[0], ab)
		s := n.newState()
		a := n.newState()
		n.addEdge(s, -1, g.start)
		n.addEdge(s, -1, a)
		n.addEdge(g.accept, -1, a)
		return frag{s, a}
	default:
		// OpEmptyMatch, anchors, and any op that contributes no input symbols
		// are treated as epsilon.
		return n.epsilonFrag()
	}
}

func (n *nfa) literalFrag(runes []rune, ab *alphabet) frag {
	s := n.newState()
	cur := s
	for _, r := range runes {
		if r < 256 {
			nx := n.newState()
			for _, sym := range ab.symsForRange(int(r), int(r)) {
				n.addEdge(cur, sym, nx)
			}
			cur = nx
		} else {
			for _, b := range []byte(string(r)) {
				nx := n.newState()
				for _, sym := range ab.symsForRange(int(b), int(b)) {
					n.addEdge(cur, sym, nx)
				}
				cur = nx
			}
		}
	}
	return frag{s, cur}
}

func (n *nfa) symsFrag(syms []int) frag {
	s := n.newState()
	a := n.newState()
	for _, sym := range syms {
		n.addEdge(s, sym, a)
	}
	return frag{s, a}
}
