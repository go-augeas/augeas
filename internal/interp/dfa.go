// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// dfa is a deterministic automaton over the shared symbol alphabet. trans[s][k]
// is the target state for symbol k, or -1 for no transition (implicit dead
// state).
type dfa struct {
	ab     *alphabet
	trans  [][]int
	accept []bool
	start  int
}

func (n *nfa) epsClosure(states map[int]bool) {
	stack := make([]int, 0, len(states))
	for s := range states {
		stack = append(stack, s)
	}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, t := range n.trans[s][-1] {
			if !states[t] {
				states[t] = true
				stack = append(stack, t)
			}
		}
	}
}

func setKey(states map[int]bool) string {
	ks := make([]int, 0, len(states))
	for s := range states {
		ks = append(ks, s)
	}
	sort.Ints(ks)
	var b strings.Builder
	for _, k := range ks {
		b.WriteString(strconv.Itoa(k))
		b.WriteByte(',')
	}
	return b.String()
}

// determinize converts an NFA to a DFA by subset construction.
func (n *nfa) determinize(ab *alphabet) *dfa {
	nsym := ab.nsym()
	d := &dfa{ab: ab}
	index := map[string]int{}
	var sets []map[int]bool

	startSet := map[int]bool{n.start: true}
	n.epsClosure(startSet)
	index[setKey(startSet)] = 0
	sets = append(sets, startSet)
	d.trans = append(d.trans, nil)
	d.accept = append(d.accept, startSet[n.accept])
	d.start = 0

	for i := 0; i < len(sets); i++ {
		set := sets[i]
		row := make([]int, nsym)
		for k := 0; k < nsym; k++ {
			next := map[int]bool{}
			for s := range set {
				for _, t := range n.trans[s][k] {
					next[t] = true
				}
			}
			if len(next) == 0 {
				row[k] = -1
				continue
			}
			n.epsClosure(next)
			key := setKey(next)
			id, ok := index[key]
			if !ok {
				id = len(sets)
				index[key] = id
				sets = append(sets, next)
				d.trans = append(d.trans, nil)
				d.accept = append(d.accept, next[n.accept])
			}
			row[k] = id
		}
		d.trans[i] = row
	}
	return d
}

// complete adds an explicit dead state so every (state,symbol) has a target.
func (d *dfa) complete() {
	nsym := d.ab.nsym()
	dead := len(d.trans)
	deadRow := make([]int, nsym)
	for k := range deadRow {
		deadRow[k] = dead
	}
	changed := false
	for i := range d.trans {
		if d.trans[i] == nil {
			d.trans[i] = make([]int, nsym)
			for k := range d.trans[i] {
				d.trans[i][k] = -1
			}
		}
		for k := 0; k < nsym; k++ {
			if d.trans[i][k] == -1 {
				d.trans[i][k] = dead
				changed = true
			}
		}
	}
	if changed {
		d.trans = append(d.trans, deadRow)
		d.accept = append(d.accept, false)
	}
}

// complement returns the complement DFA (accepting exactly the strings the
// original rejects) over the alphabet.
func (d *dfa) complement() *dfa {
	d.complete()
	acc := make([]bool, len(d.accept))
	for i := range d.accept {
		acc[i] = !d.accept[i]
	}
	nd := &dfa{ab: d.ab, trans: d.trans, accept: acc, start: d.start}
	return nd
}

// intersect returns the product DFA accepting L(d) ∩ L(o).
func (d *dfa) intersect(o *dfa) *dfa {
	d.complete()
	o.complete()
	nsym := d.ab.nsym()
	res := &dfa{ab: d.ab}
	index := map[[2]int]int{}
	type pair = [2]int
	var pairs []pair

	getID := func(p pair) int {
		if id, ok := index[p]; ok {
			return id
		}
		id := len(pairs)
		index[p] = id
		pairs = append(pairs, p)
		res.trans = append(res.trans, nil)
		res.accept = append(res.accept, d.accept[p[0]] && o.accept[p[1]])
		return id
	}
	res.start = getID(pair{d.start, o.start})
	for i := 0; i < len(pairs); i++ {
		p := pairs[i]
		row := make([]int, nsym)
		for k := 0; k < nsym; k++ {
			row[k] = getID(pair{d.trans[p[0]][k], o.trans[p[1]][k]})
		}
		res.trans[i] = row
	}
	return res
}

// reachableAccept reports whether any accepting state is reachable from start.
func (d *dfa) nonEmpty() bool {
	seen := make([]bool, len(d.trans))
	stack := []int{d.start}
	seen[d.start] = true
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if d.accept[s] {
			return true
		}
		for _, t := range d.trans[s] {
			if t >= 0 && !seen[t] {
				seen[t] = true
				stack = append(stack, t)
			}
		}
	}
	return false
}

// toRegexp converts the DFA to an equivalent regexp pattern via GNFA state
// elimination. It returns an error if the language is empty (which upstream
// cannot represent as a regexp either).
func (d *dfa) toRegexp() (string, error) {
	if !d.nonEmpty() {
		return "", fmt.Errorf("regexp difference is the empty set")
	}
	// Keep only states reachable and co-reachable to bound the size.
	m := len(d.trans)
	start := m
	accept := m + 1
	n := m + 2

	// R[i][j] as extended regex (nil = ∅, "" = epsilon).
	R := make([][]*string, n)
	for i := range R {
		R[i] = make([]*string, n)
	}
	empty := ""
	R[start][d.start] = &empty
	for s := 0; s < m; s++ {
		if d.accept[s] {
			R[s][accept] = &empty
		}
	}
	// Edge labels: gather symbols per (i,j).
	for i := 0; i < m; i++ {
		bySym := map[int][]int{} // target -> symbols
		for k, t := range d.trans[i] {
			if t >= 0 {
				bySym[t] = append(bySym[t], k)
			}
		}
		for t, syms := range bySym {
			cc := d.charClass(syms)
			R[i][t] = reUnion(R[i][t], &cc)
		}
	}

	// Eliminate real states 0..m-1.
	for k := 0; k < m; k++ {
		kk := reStar(R[k][k])
		for i := 0; i < n; i++ {
			if i == k || R[i][k] == nil {
				continue
			}
			for j := 0; j < n; j++ {
				if j == k || R[k][j] == nil {
					continue
				}
				add := reConcat(R[i][k], reConcat(kk, R[k][j]))
				R[i][j] = reUnion(R[i][j], add)
			}
		}
		for i := 0; i < n; i++ {
			R[i][k] = nil
			R[k][i] = nil
		}
	}

	if R[start][accept] == nil {
		return "", fmt.Errorf("regexp difference is the empty set")
	}
	return *R[start][accept], nil
}

// charClass renders a set of alphabet symbols as an RE2 character class. The
// output is raw RE2 (the automata results bypass toRE2), so it may use RE2
// escapes freely, including \xNN for bytes outside the printable ASCII range,
// which keeps the pattern valid UTF-8.
func (d *dfa) charClass(syms []int) string {
	type rng struct{ lo, hi int }
	var rs []rng
	sort.Ints(syms)
	for _, s := range syms {
		lo, hi := d.ab.interval(s)
		if len(rs) > 0 && rs[len(rs)-1].hi+1 == lo {
			rs[len(rs)-1].hi = hi
		} else {
			rs = append(rs, rng{lo, hi})
		}
	}
	var b strings.Builder
	b.WriteByte('[')
	for _, r := range rs {
		if r.lo == r.hi {
			b.WriteString(ccByte(byte(r.lo)))
		} else {
			b.WriteString(ccByte(byte(r.lo)))
			b.WriteByte('-')
			b.WriteString(ccByte(byte(r.hi)))
		}
	}
	b.WriteByte(']')
	return b.String()
}

// ccByte renders a byte for inclusion in an RE2 character class.
func ccByte(b byte) string {
	switch b {
	case ']', '\\', '^', '-':
		return "\\" + string(b)
	}
	if b < 0x20 || b > 0x7e {
		return fmt.Sprintf("\\x%02x", b)
	}
	return string(b)
}

// Extended-regexp string combinators. nil represents the empty language (∅);
// the empty string represents epsilon. Unions and stars always produce atomic
// (parenthesised) results so concatenation can simply juxtapose.

func reUnion(a, b *string) *string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *a == *b {
		return a
	}
	s := "(?:" + *a + "|" + *b + ")"
	return &s
}

func reConcat(a, b *string) *string {
	if a == nil || b == nil {
		return nil
	}
	if *a == "" {
		return b
	}
	if *b == "" {
		return a
	}
	s := *a + *b
	return &s
}

func reStar(a *string) *string {
	empty := ""
	if a == nil || *a == "" {
		return &empty
	}
	s := "(?:" + *a + ")*"
	return &s
}
