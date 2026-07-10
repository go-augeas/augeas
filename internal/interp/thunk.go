// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "fmt"

// thunk is a lazily-evaluated module binding. It captures the binding's
// expression and defining environment and evaluates on first force, caching the
// result (or error).
type thunk struct {
	i       *interp
	term    term
	env     *env
	recName string
	rec     bool

	forcing bool
	done    bool
	val     Value
	err     error
}

func (*thunk) isValue() {}

func (t *thunk) force() (Value, error) {
	if t.done {
		return t.val, t.err
	}
	if t.forcing {
		return nil, fmt.Errorf("binding %s depends on itself", t.recName)
	}
	t.forcing = true
	if t.rec {
		t.val, t.err = t.i.evalRec(t.recName, t.term, t.env)
	} else {
		t.val, t.err = t.i.eval(t.term, t.env)
	}
	t.forcing = false
	t.done = true
	return t.val, t.err
}

// force resolves a value if it is a thunk, otherwise returns it unchanged.
func force(v Value) (Value, error) {
	if t, ok := v.(*thunk); ok {
		return t.force()
	}
	return v, nil
}
