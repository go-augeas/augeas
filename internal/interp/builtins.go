// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"os"
)

// registerBuiltins seeds the global environment with the native functions the
// lens language exposes (the "builtin" module in upstream Augeas).
func registerBuiltins(i *interp) {
	reg := func(name string, arity int, fn func(*interp, []Value) (Value, error)) {
		i.global.bind(name, &vNative{name: name, arity: arity, fn: fn})
	}

	reg("del", 2, func(_ *interp, a []Value) (Value, error) {
		re, ok := asRegexp(a[0])
		if !ok {
			return nil, fmt.Errorf("del: first argument must be a regexp")
		}
		def, ok := a[1].(vString)
		if !ok {
			return nil, fmt.Errorf("del: second argument must be a string")
		}
		return &vLens{lens: makePrim(lDel, re, string(def))}, nil
	})
	reg("store", 1, func(_ *interp, a []Value) (Value, error) {
		re, ok := asRegexp(a[0])
		if !ok {
			return nil, fmt.Errorf("store: argument must be a regexp")
		}
		return &vLens{lens: makePrim(lStore, re, "")}, nil
	})
	reg("key", 1, func(_ *interp, a []Value) (Value, error) {
		re, ok := asRegexp(a[0])
		if !ok {
			return nil, fmt.Errorf("key: argument must be a regexp")
		}
		return &vLens{lens: makePrim(lKey, re, "")}, nil
	})
	reg("label", 1, func(_ *interp, a []Value) (Value, error) {
		s, ok := a[0].(vString)
		if !ok {
			return nil, fmt.Errorf("label: argument must be a string")
		}
		return &vLens{lens: makePrim(lLabel, nil, string(s))}, nil
	})
	reg("value", 1, func(_ *interp, a []Value) (Value, error) {
		s, ok := a[0].(vString)
		if !ok {
			return nil, fmt.Errorf("value: argument must be a string")
		}
		return &vLens{lens: makePrim(lValue, nil, string(s))}, nil
	})
	reg("seq", 1, func(_ *interp, a []Value) (Value, error) {
		s, ok := a[0].(vString)
		if !ok {
			return nil, fmt.Errorf("seq: argument must be a string")
		}
		return &vLens{lens: makePrim(lSeq, nil, string(s))}, nil
	})
	reg("counter", 1, func(_ *interp, a []Value) (Value, error) {
		s, ok := a[0].(vString)
		if !ok {
			return nil, fmt.Errorf("counter: argument must be a string")
		}
		return &vLens{lens: makePrim(lCounter, nil, string(s))}, nil
	})
	reg("square", 3, func(_ *interp, a []Value) (Value, error) {
		l1, ok1 := a[0].(*vLens)
		l2, ok2 := a[1].(*vLens)
		l3, ok3 := a[2].(*vLens)
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("square: arguments must be lenses")
		}
		return &vLens{lens: makeSquare(l1.lens, l2.lens, l3.lens)}, nil
	})

	reg("incl", 1, func(_ *interp, a []Value) (Value, error) {
		s, ok := a[0].(vString)
		if !ok {
			return nil, fmt.Errorf("incl: argument must be a string")
		}
		return &vFilter{filter: []filterEntry{{glob: string(s), include: true}}}, nil
	})
	reg("excl", 1, func(_ *interp, a []Value) (Value, error) {
		s, ok := a[0].(vString)
		if !ok {
			return nil, fmt.Errorf("excl: argument must be a string")
		}
		return &vFilter{filter: []filterEntry{{glob: string(s), include: false}}}, nil
	})
	reg("transform", 2, func(_ *interp, a []Value) (Value, error) {
		l, ok := a[0].(*vLens)
		if !ok {
			return nil, fmt.Errorf("transform: first argument must be a lens")
		}
		f, ok := a[1].(*vFilter)
		if !ok {
			return nil, fmt.Errorf("transform: second argument must be a filter")
		}
		return &vTransform{lens: l.lens, filter: f.filter}, nil
	})

	// Diagnostics / identity natives used by some modules.
	reg("get", 2, func(ip *interp, a []Value) (Value, error) {
		l, ok := a[0].(*vLens)
		s, ok2 := a[1].(vString)
		if !ok || !ok2 {
			return nil, fmt.Errorf("get: bad arguments")
		}
		forest, err := LnsGet(l.lens, string(s))
		if err != nil {
			return nil, err
		}
		return &vTree{forest: forest}, nil
	})
	reg("print_string", 1, func(_ *interp, a []Value) (Value, error) { return vUnit{}, nil })
	reg("print_regexp", 1, func(_ *interp, a []Value) (Value, error) { return vUnit{}, nil })
	reg("print_endline", 1, func(_ *interp, a []Value) (Value, error) { return vUnit{}, nil })
	reg("print_tree", 1, func(_ *interp, a []Value) (Value, error) { return a[0], nil })

	reg("lens_ctype", 1, lensType(func(l *Lens) *Regexp { return l.ctype }))
	reg("lens_atype", 1, lensType(func(l *Lens) *Regexp { return l.ctype }))
	reg("lens_ktype", 1, lensType(func(l *Lens) *Regexp { return l.ctype }))
	reg("lens_vtype", 1, lensType(func(l *Lens) *Regexp { return l.ctype }))

	// The Sys module functions are registered under qualified names by making
	// a "Sys" module available implicitly.
	i.global.bind("Sys.getenv", &vNative{name: "Sys.getenv", arity: 1, fn: func(_ *interp, a []Value) (Value, error) {
		s, _ := a[0].(vString)
		return vString(os.Getenv(string(s))), nil
	}})
}

func lensType(sel func(*Lens) *Regexp) func(*interp, []Value) (Value, error) {
	return func(_ *interp, a []Value) (Value, error) {
		l, ok := a[0].(*vLens)
		if !ok {
			return nil, fmt.Errorf("lens type: argument must be a lens")
		}
		return &vRegexp{re: sel(l.lens)}, nil
	}
}
