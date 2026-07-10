// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"strings"
)

// Source resolves a module's source text by lowercased module name (without the
// ".aug" suffix), e.g. "hosts" for module Hosts.
type Source func(base string) (string, bool)

// interp is the interpreter state: the source resolver, the global environment
// (native builtins) and the cache of loaded modules.
type interp struct {
	source  Source
	global  *env
	modules map[string]*module
	loading map[string]bool
}

type module struct {
	name     string
	env      *env
	autoload string
	tests    []*testTerm
}

// New creates an interpreter that resolves module sources via src.
func New(src Source) *interp {
	i := &interp{
		source:  src,
		global:  newEnv(nil),
		modules: map[string]*module{},
		loading: map[string]bool{},
	}
	registerBuiltins(i)
	registerTreeCmds(i)
	return i
}

// LoadModule loads and evaluates the module with the given name (e.g. "Hosts").
func (i *interp) LoadModule(name string) (*module, error) {
	if m, ok := i.modules[name]; ok {
		return m, nil
	}
	if i.loading[name] {
		return nil, fmt.Errorf("module import cycle at %s", name)
	}
	i.loading[name] = true
	defer delete(i.loading, name)

	base := strings.ToLower(name)
	src, ok := i.source(base)
	if !ok {
		return nil, fmt.Errorf("module %s not found (%s.aug)", name, base)
	}
	mod, err := parseModule(src)
	if err != nil {
		return nil, fmt.Errorf("module %s: %w", name, err)
	}
	if !strings.EqualFold(mod.name, name) {
		return nil, fmt.Errorf("module %s declares name %s", name, mod.name)
	}
	m := &module{name: mod.name, env: newEnv(i.global), autoload: mod.autoload}
	i.modules[name] = m
	// Bindings are evaluated lazily: a binding that fails to compile (for
	// example because it uses an as-yet-unsupported feature) must not prevent
	// the rest of the module — or modules that import it but do not use that
	// binding — from loading. This mirrors nothing in upstream, which is eager,
	// but preserves behavioural compatibility for every binding that is used.
	for _, d := range mod.decls {
		switch dt := d.(type) {
		case *bindTerm:
			m.env.bind(dt.name, &thunk{i: i, term: dt.exp, env: m.env})
		case *bindRecTerm:
			m.env.bind(dt.name, &thunk{i: i, recName: dt.name, term: dt.exp, env: m.env, rec: true})
		case *testTerm:
			m.tests = append(m.tests, dt)
		}
	}
	return m, nil
}

// Lookup returns the value of a binding within a module.
func (i *interp) Lookup(moduleName, binding string) (Value, error) {
	m, err := i.LoadModule(moduleName)
	if err != nil {
		return nil, err
	}
	v, ok := m.env.lookup(binding)
	if !ok {
		return nil, fmt.Errorf("module %s has no binding %s", moduleName, binding)
	}
	return force(v)
}

func (i *interp) eval(t term, e *env) (Value, error) {
	switch x := t.(type) {
	case *stringTerm:
		return vString(x.value), nil
	case *regexpTerm:
		return &vRegexp{re: newRegexp(x.pattern, x.nocase)}, nil
	case *unitTerm:
		return vUnit{}, nil
	case *identTerm:
		return i.evalIdent(x.name, e)
	case *funcTerm:
		return &vClosure{param: x.param.name, body: x.body, env: e}, nil
	case *letTerm:
		v, err := i.eval(buildFunc(x.params, x.exp), e)
		if err != nil {
			return nil, err
		}
		ne := newEnv(e)
		ne.bind(x.name, v)
		return i.eval(x.body, ne)
	case *bracketTerm:
		v, err := i.eval(x.exp, e)
		if err != nil {
			return nil, err
		}
		l, ok := v.(*vLens)
		if !ok {
			return nil, fmt.Errorf("[ ] requires a lens")
		}
		return &vLens{lens: makeSubtree(l.lens)}, nil
	case *repTerm:
		return i.evalRep(x, e)
	case *binopTerm:
		return i.evalBinop(x, e)
	case *treeValueTerm:
		return &vTree{forest: buildTreeLit(x.nodes)}, nil
	}
	return nil, fmt.Errorf("cannot evaluate term %T", t)
}

func (i *interp) evalIdent(name string, e *env) (Value, error) {
	if v, ok := e.lookup(name); ok {
		return force(v)
	}
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		mod := name[:dot]
		bind := name[dot+1:]
		return i.Lookup(mod, bind)
	}
	return nil, fmt.Errorf("unbound identifier %q", name)
}

func (i *interp) evalRep(x *repTerm, e *env) (Value, error) {
	v, err := i.eval(x.exp, e)
	if err != nil {
		return nil, err
	}
	if l, ok := v.(*vLens); ok {
		switch x.quant {
		case qStar:
			return &vLens{lens: makeStar(l.lens)}, nil
		case qPlus:
			return &vLens{lens: makePlus(l.lens)}, nil
		default:
			return &vLens{lens: makeMaybe(l.lens)}, nil
		}
	}
	re, ok := asRegexp(v)
	if !ok {
		return nil, fmt.Errorf("repetition requires a lens or regexp")
	}
	switch x.quant {
	case qStar:
		return &vRegexp{re: regexpIter(re, 0, -1)}, nil
	case qPlus:
		return &vRegexp{re: regexpIter(re, 1, -1)}, nil
	default:
		return &vRegexp{re: regexpMaybe(re)}, nil
	}
}

func (i *interp) evalBinop(x *binopTerm, e *env) (Value, error) {
	if x.tag == opApp {
		return i.evalApp(x, e)
	}
	if x.tag == opCompose {
		return i.evalCompose(x, e)
	}
	l, err := i.eval(x.left, e)
	if err != nil {
		return nil, err
	}
	r, err := i.eval(x.right, e)
	if err != nil {
		return nil, err
	}
	switch x.tag {
	case opConcat:
		return concatValues(l, r)
	case opUnion:
		return unionValues(l, r)
	case opMinus:
		return minusValues(l, r)
	}
	return nil, fmt.Errorf("unknown binop")
}

func (i *interp) evalApp(x *binopTerm, e *env) (Value, error) {
	fn, err := i.eval(x.left, e)
	if err != nil {
		return nil, err
	}
	arg, err := i.eval(x.right, e)
	if err != nil {
		return nil, err
	}
	return i.apply(fn, arg)
}

func (i *interp) apply(fn, arg Value) (Value, error) {
	switch f := fn.(type) {
	case *vClosure:
		ne := newEnv(f.env)
		ne.bind(f.param, arg)
		return i.eval(f.body, ne)
	case *vNative:
		args := append(append([]Value{}, f.args...), arg)
		if len(args) < f.arity {
			return &vNative{name: f.name, arity: f.arity, args: args, fn: f.fn}, nil
		}
		return f.fn(i, args)
	case *composed:
		mid, err := i.apply(f.f, arg)
		if err != nil {
			return nil, err
		}
		return i.apply(f.g, mid)
	}
	return nil, fmt.Errorf("cannot apply non-function %T", fn)
}

func (i *interp) evalCompose(x *binopTerm, e *env) (Value, error) {
	// Used both for sequencing tree commands (set ; rm) and for function
	// composition. When the left side is a function we build a composed
	// closure; otherwise we evaluate left for its effect and return right.
	l, err := i.eval(x.left, e)
	if err != nil {
		return nil, err
	}
	if isFunc(l) {
		r, err := i.eval(x.right, e)
		if err != nil {
			return nil, err
		}
		return &composed{f: l, g: r, i: i}, nil
	}
	return i.eval(x.right, e)
}

func isFunc(v Value) bool {
	switch v.(type) {
	case *vClosure, *vNative, *composed:
		return true
	}
	return false
}

// composed represents g . f as a one-argument function x -> g (f x).
type composed struct {
	f, g Value
	i    *interp
}

func (*composed) isValue() {}

func concatValues(l, r Value) (Value, error) {
	if ll, ok := l.(*vLens); ok {
		rl, ok := r.(*vLens)
		if !ok {
			return nil, fmt.Errorf("cannot concat lens with %T", r)
		}
		return &vLens{lens: makeConcat(ll.lens, rl.lens)}, nil
	}
	if _, ok := r.(*vLens); ok {
		return nil, fmt.Errorf("cannot concat %T with lens", l)
	}
	if lf, ok := l.(*vFilter); ok {
		rf, ok := r.(*vFilter)
		if !ok {
			return nil, fmt.Errorf("cannot concat filter with %T", r)
		}
		return &vFilter{filter: append(append([]filterEntry{}, lf.filter...), rf.filter...)}, nil
	}
	ls, lIsStr := l.(vString)
	rs, rIsStr := r.(vString)
	if lIsStr && rIsStr {
		return vString(string(ls) + string(rs)), nil
	}
	lre, ok1 := asRegexp(l)
	rre, ok2 := asRegexp(r)
	if ok1 && ok2 {
		return &vRegexp{re: regexpConcat(lre, rre)}, nil
	}
	return nil, fmt.Errorf("cannot concat %T and %T", l, r)
}

func unionValues(l, r Value) (Value, error) {
	if ll, ok := l.(*vLens); ok {
		rl, ok := r.(*vLens)
		if !ok {
			return nil, fmt.Errorf("cannot union lens with %T", r)
		}
		return &vLens{lens: makeUnion(ll.lens, rl.lens)}, nil
	}
	if _, ok := r.(*vLens); ok {
		return nil, fmt.Errorf("cannot union %T with lens", l)
	}
	lre, ok1 := asRegexp(l)
	rre, ok2 := asRegexp(r)
	if ok1 && ok2 {
		return &vRegexp{re: regexpUnion(lre, rre)}, nil
	}
	return nil, fmt.Errorf("cannot union %T and %T", l, r)
}

func minusValues(l, r Value) (Value, error) {
	lre, ok1 := asRegexp(l)
	rre, ok2 := asRegexp(r)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("cannot subtract %T and %T", l, r)
	}
	re, err := regexpMinus(lre, rre)
	if err != nil {
		return nil, err
	}
	return &vRegexp{re: re}, nil
}

func buildTreeLit(nodes []*treeLit) []*Tree {
	var out []*Tree
	for _, n := range nodes {
		t := &Tree{Children: buildTreeLit(n.children)}
		if n.label != nil {
			t.Label = strptr(*n.label)
		}
		if n.value != nil {
			t.Value = strptr(*n.value)
		}
		out = append(out, t)
	}
	return out
}
