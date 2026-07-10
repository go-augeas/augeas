// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "fmt"

type parser struct {
	toks []token
	pos  int
}

func parseModule(src string) (*moduleTerm, error) {
	lx := newLexer(src)
	toks, err := lx.tokens()
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	return p.parseStart()
}

func (p *parser) peek() token       { return p.toks[p.pos] }
func (p *parser) at(k tokKind) bool { return p.toks[p.pos].kind == k }

func (p *parser) advance() token {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}

func (p *parser) expect(k tokKind) (token, error) {
	if p.toks[p.pos].kind != k {
		return token{}, fmt.Errorf("line %d: expected token %d, got %d",
			p.toks[p.pos].line, k, p.toks[p.pos].kind)
	}
	return p.advance(), nil
}

func (p *parser) parseStart() (*moduleTerm, error) {
	if _, err := p.expect(tModule); err != nil {
		return nil, err
	}
	name, err := p.expect(tUIdent)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tEq); err != nil {
		return nil, err
	}
	m := &moduleTerm{name: name.sval}
	if p.at(tAutoload) {
		p.advance()
		al, err := p.expect(tLIdent)
		if err != nil {
			return nil, err
		}
		m.autoload = al.sval
	}
	decls, err := p.parseDecls()
	if err != nil {
		return nil, err
	}
	m.decls = decls
	if !p.at(tEOF) {
		return nil, fmt.Errorf("line %d: trailing tokens after module", p.peek().line)
	}
	return m, nil
}

func (p *parser) parseDecls() ([]term, error) {
	var decls []term
	for {
		switch p.peek().kind {
		case tLet:
			p.advance()
			name, err := p.expect(tLIdent)
			if err != nil {
				return nil, err
			}
			params, err := p.parseParamList()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tEq); err != nil {
				return nil, err
			}
			exp, err := p.parseExp()
			if err != nil {
				return nil, err
			}
			decls = append(decls, &bindTerm{name: name.sval, exp: buildFunc(params, exp)})
		case tLetRec:
			p.advance()
			name, err := p.expect(tLIdent)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tEq); err != nil {
				return nil, err
			}
			exp, err := p.parseExp()
			if err != nil {
				return nil, err
			}
			decls = append(decls, &bindRecTerm{name: name.sval, exp: exp})
		case tTest:
			line := p.peek().line
			p.advance()
			texp, err := p.parseTestExp()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tEq); err != nil {
				return nil, err
			}
			t := &testTerm{exp: texp, line: line}
			switch p.peek().kind {
			case tQuestion:
				p.advance()
				t.tag = trPrint
			case tStar:
				p.advance()
				t.tag = trExn
			default:
				res, err := p.parseExp()
				if err != nil {
					return nil, err
				}
				t.tag = trCheck
				t.result = res
			}
			decls = append(decls, t)
		default:
			return decls, nil
		}
	}
}

func (p *parser) parseTestExp() (term, error) {
	lens, err := p.parseAExp()
	if err != nil {
		return nil, err
	}
	switch p.peek().kind {
	case tGet:
		p.advance()
		arg, err := p.parseExp()
		if err != nil {
			return nil, err
		}
		return &getTestTerm{lens: lens, arg: arg}, nil
	case tPut:
		p.advance()
		arg, err := p.parseAExp()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tAfter); err != nil {
			return nil, err
		}
		cmds, err := p.parseExp()
		if err != nil {
			return nil, err
		}
		return &putTestTerm{lens: lens, arg: arg, cmds: cmds}, nil
	}
	return nil, fmt.Errorf("line %d: expected get/put in test", p.peek().line)
}

func (p *parser) parseParamList() ([]param, error) {
	var params []param
	for p.at(tLParen) {
		// distinguish "(" id ":" ... from "(" exp ")" — a param always has id ':'
		if p.toks[p.pos+1].kind != tLIdent && p.toks[p.pos+1].kind != tGet && p.toks[p.pos+1].kind != tPut {
			break
		}
		if p.toks[p.pos+2].kind != tColon {
			break
		}
		p.advance() // (
		id := p.advance()
		p.advance() // :
		if err := p.skipType(); err != nil {
			return nil, err
		}
		if _, err := p.expect(tRParen); err != nil {
			return nil, err
		}
		params = append(params, param{name: idName(id)})
	}
	return params, nil
}

func idName(t token) string {
	switch t.kind {
	case tGet:
		return "get"
	case tPut:
		return "put"
	default:
		return t.sval
	}
}

// skipType consumes a type expression (atype (-> type)*), discarding it.
func (p *parser) skipType() error {
	if err := p.skipAType(); err != nil {
		return err
	}
	for p.at(tArrow) {
		p.advance()
		if err := p.skipAType(); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) skipAType() error {
	switch p.peek().kind {
	case tString, tRegexp, tLens:
		p.advance()
		return nil
	case tLParen:
		p.advance()
		if err := p.skipType(); err != nil {
			return err
		}
		_, err := p.expect(tRParen)
		return err
	}
	return fmt.Errorf("line %d: expected type", p.peek().line)
}

func buildFunc(params []param, body term) term {
	for i := len(params) - 1; i >= 0; i-- {
		body = &funcTerm{param: params[i], body: body}
	}
	return body
}

func (p *parser) parseExp() (term, error) {
	if p.at(tLet) {
		p.advance()
		name, err := p.expect(tLIdent)
		if err != nil {
			return nil, err
		}
		params, err := p.parseParamList()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tEq); err != nil {
			return nil, err
		}
		exp, err := p.parseExp()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tIn); err != nil {
			return nil, err
		}
		body, err := p.parseExp()
		if err != nil {
			return nil, err
		}
		return &letTerm{name: name.sval, params: params, exp: exp, body: body}, nil
	}
	return p.parseCompose()
}

func (p *parser) parseCompose() (term, error) {
	left, err := p.parseUnion()
	if err != nil {
		return nil, err
	}
	for p.at(tSemi) {
		p.advance()
		right, err := p.parseUnion()
		if err != nil {
			return nil, err
		}
		left = &binopTerm{tag: opCompose, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnion() (term, error) {
	var left term
	var err error
	if p.at(tLBrace) {
		left, err = p.parseTreeConst()
	} else {
		left, err = p.parseMinus()
	}
	if err != nil {
		return nil, err
	}
	for p.at(tPipe) {
		p.advance()
		var right term
		if p.at(tLBrace) {
			right, err = p.parseTreeConst()
		} else {
			right, err = p.parseMinus()
		}
		if err != nil {
			return nil, err
		}
		left = &binopTerm{tag: opUnion, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseMinus() (term, error) {
	left, err := p.parseCat()
	if err != nil {
		return nil, err
	}
	for p.at(tMinus) {
		p.advance()
		right, err := p.parseCat()
		if err != nil {
			return nil, err
		}
		left = &binopTerm{tag: opMinus, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseCat() (term, error) {
	left, err := p.parseApp()
	if err != nil {
		return nil, err
	}
	for p.at(tDot) {
		p.advance()
		right, err := p.parseApp()
		if err != nil {
			return nil, err
		}
		left = &binopTerm{tag: opConcat, left: left, right: right}
	}
	return left, nil
}

func (p *parser) startsAExp() bool {
	switch p.peek().kind {
	case tLIdent, tQIdent, tGet, tPut, tDQuoted, tRegexpLit, tLParen, tLBracket:
		return true
	}
	return false
}

func (p *parser) parseApp() (term, error) {
	left, err := p.parseRexp()
	if err != nil {
		return nil, err
	}
	for p.startsAExp() {
		right, err := p.parseRexp()
		if err != nil {
			return nil, err
		}
		left = &binopTerm{tag: opApp, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseRexp() (term, error) {
	a, err := p.parseAExp()
	if err != nil {
		return nil, err
	}
	switch p.peek().kind {
	case tStar:
		p.advance()
		return &repTerm{exp: a, quant: qStar}, nil
	case tPlus:
		p.advance()
		return &repTerm{exp: a, quant: qPlus}, nil
	case tQuestion:
		p.advance()
		return &repTerm{exp: a, quant: qMaybe}, nil
	}
	return a, nil
}

func (p *parser) parseAExp() (term, error) {
	t := p.peek()
	switch t.kind {
	case tLIdent, tQIdent:
		p.advance()
		return &identTerm{name: t.sval}, nil
	case tGet:
		p.advance()
		return &identTerm{name: "get"}, nil
	case tPut:
		p.advance()
		return &identTerm{name: "put"}, nil
	case tDQuoted:
		p.advance()
		return &stringTerm{value: t.sval}, nil
	case tRegexpLit:
		p.advance()
		return &regexpTerm{pattern: t.sval, nocase: t.nocase}, nil
	case tLParen:
		p.advance()
		if p.at(tRParen) {
			p.advance()
			return &unitTerm{}, nil
		}
		e, err := p.parseExp()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRParen); err != nil {
			return nil, err
		}
		return e, nil
	case tLBracket:
		p.advance()
		e, err := p.parseExp()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRBracket); err != nil {
			return nil, err
		}
		return &bracketTerm{exp: e}, nil
	}
	return nil, fmt.Errorf("line %d: unexpected token %d in expression", t.line, t.kind)
}

func (p *parser) parseTreeConst() (term, error) {
	tv := &treeValueTerm{}
	for p.at(tLBrace) {
		p.advance()
		br, err := p.parseTreeBranch()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRBrace); err != nil {
			return nil, err
		}
		tv.nodes = append(tv.nodes, br)
	}
	if len(tv.nodes) == 0 {
		return nil, fmt.Errorf("line %d: empty tree constant", p.peek().line)
	}
	return tv, nil
}

func (p *parser) parseTreeBranch() (*treeLit, error) {
	tl := &treeLit{}
	if p.at(tDQuoted) {
		lbl := p.advance().sval
		tl.label = &lbl
	}
	if p.at(tEq) {
		p.advance()
		v, err := p.expect(tDQuoted)
		if err != nil {
			return nil, err
		}
		val := v.sval
		tl.value = &val
	}
	for p.at(tLBrace) {
		p.advance()
		child, err := p.parseTreeBranch()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRBrace); err != nil {
			return nil, err
		}
		tl.children = append(tl.children, child)
	}
	return tl, nil
}
