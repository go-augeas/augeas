// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

// Value is a runtime value of the lens language. Concrete kinds are string,
// regexp, lens, tree (a forest), filter, unit, and functions (closures and
// partially applied natives).
type Value interface{ isValue() }

type vString string

type vRegexp struct{ re *Regexp }

type vLens struct{ lens *Lens }

type vTree struct{ forest []*Tree }

type vFilter struct{ filter []filterEntry }

type filterEntry struct {
	glob    string
	include bool
}

type vUnit struct{}

// vTransform is the result of `transform lens filter`, carrying the autoload
// lens together with its include/exclude filter for the Load mechanism.
type vTransform struct {
	lens   *Lens
	filter []filterEntry
}

// vClosure is a user-defined function value.
type vClosure struct {
	param string
	body  term
	env   *env
}

// vNative is a builtin function, possibly partially applied.
type vNative struct {
	name  string
	arity int
	args  []Value
	fn    func(i *interp, args []Value) (Value, error)
}

func (vString) isValue()     {}
func (*vRegexp) isValue()    {}
func (*vLens) isValue()      {}
func (*vTree) isValue()      {}
func (*vFilter) isValue()    {}
func (vUnit) isValue()       {}
func (*vClosure) isValue()   {}
func (*vNative) isValue()    {}
func (*vTransform) isValue() {}

// env is a lexical environment mapping names to values, with a parent scope.
type env struct {
	vars   map[string]Value
	parent *env
}

func newEnv(parent *env) *env {
	return &env{vars: map[string]Value{}, parent: parent}
}

func (e *env) lookup(name string) (Value, bool) {
	for s := e; s != nil; s = s.parent {
		if v, ok := s.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

func (e *env) bind(name string, v Value) { e.vars[name] = v }

// asRegexp coerces a value to a regexp, turning a string into a literal regexp
// (the only coercion Augeas performs).
func asRegexp(v Value) (*Regexp, bool) {
	switch x := v.(type) {
	case *vRegexp:
		return x.re, true
	case vString:
		return makeRegexpLiteral(string(x)), true
	}
	return nil, false
}
