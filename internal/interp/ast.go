// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

// binopTag identifies a binary operator term.
type binopTag int

const (
	opConcat  binopTag = iota // .
	opUnion                   // |
	opMinus                   // -
	opCompose                 // ;
	opApp                     // juxtaposition (function application)
)

// quantTag identifies a postfix repetition operator.
type quantTag int

const (
	qStar  quantTag = iota // *
	qPlus                  // +
	qMaybe                 // ?
)

// testResult classifies the expected outcome of a test declaration.
type testResult int

const (
	trCheck testResult = iota // = value : compare result
	trPrint                   // = ? : just run (debug)
	trExn                     // = * : expect the operation to fail
)

// term is a node of the parsed .aug abstract syntax tree.
type term interface{ isTerm() }

type moduleTerm struct {
	name     string
	autoload string
	decls    []term
}

type bindTerm struct {
	name string
	exp  term
}

type bindRecTerm struct {
	name string
	exp  term
}

type testTerm struct {
	exp    term // a getTestTerm or putTestTerm
	result term // expected value (may be nil for trPrint/trExn)
	tag    testResult
	line   int
}

type getTestTerm struct {
	lens term
	arg  term
}

type putTestTerm struct {
	lens term
	arg  term
	cmds term
}

type identTerm struct{ name string }

type stringTerm struct{ value string }

type regexpTerm struct {
	pattern string
	nocase  bool
}

type unitTerm struct{}

type binopTerm struct {
	tag         binopTag
	left, right term
}

type repTerm struct {
	exp   term
	quant quantTag
}

type bracketTerm struct{ exp term }

type letTerm struct {
	name   string
	params []param
	exp    term
	body   term
}

type funcTerm struct {
	param param
	body  term
}

type param struct {
	name string
}

// treeValueTerm is a tree literal { "l" = "v" ... }.
type treeValueTerm struct{ nodes []*treeLit }

type treeLit struct {
	label    *string
	value    *string
	children []*treeLit
}

func (*moduleTerm) isTerm()    {}
func (*bindTerm) isTerm()      {}
func (*bindRecTerm) isTerm()   {}
func (*testTerm) isTerm()      {}
func (*getTestTerm) isTerm()   {}
func (*putTestTerm) isTerm()   {}
func (*identTerm) isTerm()     {}
func (*stringTerm) isTerm()    {}
func (*regexpTerm) isTerm()    {}
func (*unitTerm) isTerm()      {}
func (*binopTerm) isTerm()     {}
func (*repTerm) isTerm()       {}
func (*bracketTerm) isTerm()   {}
func (*letTerm) isTerm()       {}
func (*funcTerm) isTerm()      {}
func (*treeValueTerm) isTerm() {}
