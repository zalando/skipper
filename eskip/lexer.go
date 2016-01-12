// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eskip

import (
	"errors"
	"fmt"
	"unicode"
)

type token struct {
	id  int
	val string
}

type eskipLex struct {
	code          string
	lastToken     *token
	err           error
	initialLength int
	routes        []*parsedRoute
	filters       []*Filter
}

type scanFunc func(string) (token, string, error)

type charPredicate func(byte) bool

const (
	escapeChar  = '\\'
	decimalChar = '.'
	newlineChar = '\n'
	underscore  = '_'
)

var (
	invalidCharacter = errors.New("invalid character")
	incompleteToken  = errors.New("incomplete token")
	void             = errors.New("void")
	eof              = errors.New("eof")
)

func (t token) String() string { return t.val }

func isWhitespace(c byte) bool  { return unicode.IsSpace(rune(c)) }
func isNewline(c byte) bool     { return c == newlineChar }
func isUnderscore(c byte) bool  { return c == underscore }
func isAlpha(c byte) bool       { return unicode.IsLetter(rune(c)) }
func isDigit(c byte) bool       { return unicode.IsDigit(rune(c)) }
func isSymbolChar(c byte) bool  { return isUnderscore(c) || isAlpha(c) || isDigit(c) }
func isDecimalChar(c byte) bool { return c == decimalChar }
func isNumberChar(c byte) bool  { return isDecimalChar(c) || isDigit(c) }

func scanWhile(code string, p charPredicate) ([]byte, string) {
	var b []byte
	for len(code) > 0 && p(code[0]) {
		b = append(b, code[0])
		code = code[1:]
	}

	return b, code
}

func scanVoid(code string, p charPredicate) string {
	_, rest := scanWhile(code, p)
	return rest
}

func scanFixed(id int, fixed, code string) (t token, rest string, err error) {
	if len(code) < len(fixed) || code[0:len(fixed)] != fixed {
		rest = code
		err = invalidCharacter
		return
	}

	t = token{id, fixed}
	rest = code[len(fixed):]
	return
}

func scanEscaped(delimiter byte, code string) ([]byte, string) {
	var b []byte
	escaped := false
	for len(code) > 0 {
		c := code[0]
		isDelimiter := c == delimiter
		isEscapeChar := c == escapeChar

		if escaped {
			if isDelimiter {
				b = append(b, delimiter)
				escaped = false
			} else {
				if isEscapeChar {
					b = append(b, escapeChar)
					escaped = false
				} else {
					b = append(b, escapeChar, c)
					escaped = false
				}
			}
		} else {
			if isDelimiter {
				return b, code
			} else {
				if isEscapeChar {
					escaped = true
				} else {
					b = append(b, c)
				}
			}
		}

		code = code[1:]
	}

	return b, code
}

func scanWhitespace(code string) string                     { return scanVoid(code, isWhitespace) }
func scanAnd(code string) (t token, rest string, err error) { return scanFixed(and, "&&", code) }
func scanArrow(code string) (token, string, error)          { return scanFixed(arrow, "->", code) }
func scanCloseParen(code string) (token, string, error)     { return scanFixed(closeparen, ")", code) }
func scanColon(code string) (token, string, error)          { return scanFixed(colon, ":", code) }
func scanComma(code string) (token, string, error)          { return scanFixed(comma, ",", code) }
func scanOpenParen(code string) (token, string, error)      { return scanFixed(openparen, "(", code) }
func scanSemicolon(code string) (token, string, error)      { return scanFixed(semicolon, ";", code) }
func scanShunt(code string) (token, string, error)          { return scanFixed(shunt, "<shunt>", code) }

func scanComment(code string) string {
	return scanVoid(code, func(c byte) bool { return !isNewline(c) })
}

func scanRegexpLiteral(code string) (t token, rest string, err error) {
	b, rest := scanEscaped('/', code[1:])
	if len(rest) == 0 {
		err = incompleteToken
		return
	}

	rest = rest[1:]
	t.id = regexpliteral
	t.val = string(b)
	return
}

func scanRegexpOrComment(code string) (t token, rest string, err error) {
	if len(code) < 2 {
		rest = code
		err = invalidCharacter
		return
	}

	if code[1] == '/' {
		rest = scanComment(code)
		err = void
		return
	}

	t, rest, err = scanRegexpLiteral(code)
	return
}

func scanStringLiteral(delimiter byte, code string) (t token, rest string, err error) {
	b, rest := scanEscaped(delimiter, code[1:])
	if len(rest) == 0 {
		err = incompleteToken
		return
	}

	rest = rest[1:]
	t.id = stringliteral
	t.val = string(b)
	return
}

func scanStringLiteral1(code string) (token, string, error) { return scanStringLiteral('"', code) }
func scanStringLiteral2(code string) (token, string, error) { return scanStringLiteral('`', code) }

func scanNumber(code string) (t token, rest string, err error) {
	decimal := false
	b, rest := scanWhile(code, func(c byte) bool {
		if isDecimalChar(c) {
			if decimal {
				return false
			}

			decimal = true
			return true
		}

		return isDigit(c)
	})

	t.id = number
	t.val = string(b)
	return
}

func scanSymbol(code string) (t token, rest string, err error) {
	b, rest := scanWhile(code, isSymbolChar)
	t.id = symbol
	t.val = string(b)
	return
}

func explicitCharScanner(c byte) scanFunc {
	switch c {
	case '&':
		return scanAnd
	case '-':
		return scanArrow
	case ')':
		return scanCloseParen
	case ':':
		return scanColon
	case ',':
		return scanComma
	case '(':
		return scanOpenParen
	case ';':
		return scanSemicolon
	case '<':
		return scanShunt
	case '/':
		return scanRegexpOrComment
	case '"':
		return scanStringLiteral1
	case '`':
		return scanStringLiteral2
	default:
		return nil
	}
}

func selectScan(code string) scanFunc {
	f := explicitCharScanner(code[0])
	if f != nil {
		return f
	}

	if isNumberChar(code[0]) {
		return scanNumber
	}

	if isAlpha(code[0]) || isUnderscore(code[0]) {
		return scanSymbol
	}

	return nil
}

func findScan(code string) (scan scanFunc, rest string, err error) {
	rest = scanWhitespace(code)
	if len(rest) == 0 {
		err = eof
		return
	}

	scan = selectScan(rest)
	if scan == nil {
		err = invalidCharacter
	}

	return
}

func newLexer(code string) *eskipLex {
	return &eskipLex{code: code, initialLength: len(code)}
}

func (l *eskipLex) next() (t token, err error) {
	var scan scanFunc
	scan, l.code, err = findScan(l.code)
	if err != nil {
		return
	}

	t, l.code, err = scan(l.code)
	if err == void {
		return l.next()
	}

	if err != nil {
		l.lastToken = &t
	}

	return
}

func (l *eskipLex) Lex(lval *eskipSymType) int {
	token, err := l.next()
	if err == eof {
		return -1
	}

	if err != nil {
		l.Error(err.Error())
		return -1
	}

	lval.token = token.val
	return token.id
}

func (l *eskipLex) Error(err string) {
	l.err = errors.New(fmt.Sprintf(
		"parse failed after token %v, position %d: %s",
		l.lastToken, l.initialLength-len(l.code), err))
}
