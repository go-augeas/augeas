// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "fmt"

// evalRec evaluates a `let rec name = exp` binding. It creates a recursive lens
// placeholder, binds it, evaluates the body (which may reference the
// placeholder), and ties the knot. The resulting lens is flagged recursive; the
// get direction for recursive lenses is handled in getRec.
func (i *interp) evalRec(name string, exp term, e *env) (Value, error) {
	placeholder := &Lens{tag: lRec, recursive: true, ctype: regexpMakeEmpty()}
	ne := newEnv(e)
	ne.bind(name, &vLens{lens: placeholder})
	v, err := i.eval(exp, ne)
	if err != nil {
		return nil, err
	}
	l, ok := v.(*vLens)
	if !ok {
		return nil, fmt.Errorf("let rec %s: body is not a lens", name)
	}
	placeholder.body = l.lens
	placeholder.resolved = true
	placeholder.recursive = true
	// Propagate recursiveness marker to the outer lens so get dispatches to
	// getRec.
	l.lens.recursive = true
	return l, nil
}
