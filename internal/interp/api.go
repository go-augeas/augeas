// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "fmt"

// Filter is one include/exclude glob of a transform, used by the Load
// mechanism to map files to lenses.
type Filter struct {
	Glob    string
	Include bool
}

// LensValue loads the module and returns the compiled lens bound to binding.
func (i *interp) LensValue(module, binding string) (*Lens, error) {
	v, err := i.Lookup(module, binding)
	if err != nil {
		return nil, err
	}
	l, ok := v.(*vLens)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a lens", module, binding)
	}
	return l.lens, nil
}

// Autoload loads the module and returns its autoload lens and filters, if the
// module declares an autoload transform.
func (i *interp) Autoload(module string) (*Lens, []Filter, error) {
	m, err := i.LoadModule(module)
	if err != nil {
		return nil, nil, err
	}
	if m.autoload == "" {
		return nil, nil, fmt.Errorf("module %s has no autoload", module)
	}
	v, ok := m.env.lookup(m.autoload)
	if !ok {
		return nil, nil, fmt.Errorf("module %s autoload binding %s missing", module, m.autoload)
	}
	fv, err := force(v)
	if err != nil {
		return nil, nil, err
	}
	xf, ok := fv.(*vTransform)
	if !ok {
		return nil, nil, fmt.Errorf("module %s autoload is not a transform", module)
	}
	filters := make([]Filter, 0, len(xf.filter))
	for _, f := range xf.filter {
		filters = append(filters, Filter{Glob: f.glob, Include: f.include})
	}
	return xf.lens, filters, nil
}

// Get parses text with lens into a forest.
func Get(lens *Lens, text string) ([]*Tree, error) { return LnsGet(lens, text) }

// Put serialises forest back to text with lens, reusing the skeleton parsed
// from text where the tree is unchanged.
func Put(lens *Lens, forest []*Tree, text string) (string, error) {
	return LnsPut(lens, forest, text)
}
