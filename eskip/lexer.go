package eskip

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type token struct {
	id  int
	val string
}

type charPredicate func(byte) bool

type scanner interface {
	scan(string) (token, string, error)
}

type scannerFunc func(string) (token, string, error)

func (sf scannerFunc) scan(code string) (token, string, error) { return sf(code) }

type eskipLex struct {
	code          string
	lastToken     *token
	err           error
	initialLength int
	routes        []*parsedRoute
}

type fixedScanner string

const (
	escapeChar  = '\\'
	decimalChar = '.'
	newlineChar = '\n'
	underscore  = '_'
)

var (
	invalidCharacter = errors.New("invalid character")
	incompleteToken  = errors.New("incomplete token")
	unexpectedToken  = errors.New("unexpected token")
	void             = errors.New("void")
	eof              = errors.New("eof")
)

// now this needs to be sorted
var fixedTokens = []fixedScanner{
	"&&",
	"*",
	"->",
	")",
	":",
	",",
	"(",
	";",
	"<shunt>",
	"<loopback>",
	"<dynamic>",
	"<",
	">",
}

var fixedTokenIDs = map[fixedScanner]int{
	"&&":         and,
	"*":          any,
	"->":         arrow,
	")":          closeparen,
	":":          colon,
	",":          comma,
	"(":          openparen,
	";":          semicolon,
	"<shunt>":    shunt,
	"<loopback>": loopback,
	"<dynamic>":  dynamic,
	"<":          openarrow,
	">":          closearrow,
}

func (t token) String() string { return t.val }

func (fs fixedScanner) scan(code string) (t token, rest string, err error) {
	if len(code) < len(fs) {
		err = unexpectedToken
		return
	}

	t.id = fixedTokenIDs[fs]
	t.val = string(fs)
	rest = code[len(fs):]
	return
}

func newLexer(code string) *eskipLex {
	return &eskipLex{
		code:          code,
		initialLength: len(code)}
}

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

func scanEscaped(delimiter byte, code string) ([]byte, string) {
	var b []byte
	escaped := false
	for len(code) > 0 {
		c := code[0]
		isDelimiter := c == delimiter
		isEscapeChar := c == escapeChar

		if escaped {
			if !isDelimiter && !isEscapeChar {
				b = append(b, escapeChar)
			}

			b = append(b, c)
			escaped = false
		} else {
			if isDelimiter {
				return b, code
			}

			if isEscapeChar {
				escaped = true
			} else {
				b = append(b, c)
			}
		}

		code = code[1:]
	}

	return b, code
}

func scanRegexp(code string) ([]byte, string) {
	var b []byte
	escaped := false
	var insideGroup = false
	for len(code) > 0 {
		c := code[0]
		isDelimiter := c == '/'
		isEscapeChar := c == escapeChar

		//Check if starting [... or ending ...]. Ignore if group character is escaped i.e. \[ or \]
		if !escaped && !insideGroup && c == '[' {
			insideGroup = true
		} else if !escaped && insideGroup && c == ']' {
			insideGroup = false
		}

		if escaped {
			//delimeter / is escaped in PathRegexp so that it means no end PathRegexp(/\//)
			if !isDelimiter && !isEscapeChar {
				b = append(b, escapeChar)
			}
			b = append(b, c)
			escaped = false
		} else {
			if isDelimiter && !insideGroup {
				return b, code
			}
			if isEscapeChar {
				escaped = true
			} else {
				b = append(b, c)
			}
		}
		code = code[1:]
	}
	return b, code
}

func scanRegexpLiteral(code string) (t token, rest string, err error) {
	b, rest := scanRegexp(code[1:])
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

func scanWhitespace(code string) string { return scanVoid(code, isWhitespace) }
func scanComment(code string) string {
	return scanVoid(code, func(c byte) bool { return !isNewline(c) })
}
func scanDoubleQuote(code string) (token, string, error) { return scanStringLiteral('"', code) }
func scanBacktick(code string) (token, string, error)    { return scanStringLiteral('`', code) }

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

	if isDecimalChar(b[len(b)-1]) {
		err = incompleteToken
		return
	}

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

func selectFixed(code string) scanner {
	for _, fixed := range fixedTokens {
		if len(code) >= len(fixed) && strings.HasPrefix(code, string(fixed)) {
			return fixed
		}
	}

	return nil
}

func selectVaryingScanner(code string) scanner {
	var sf scannerFunc
	switch code[0] {
	case '/':
		sf = scanRegexpOrComment
	case '"':
		sf = scanDoubleQuote
	case '`':
		sf = scanBacktick
	}

	if isNumberChar(code[0]) {
		sf = scanNumber
	}

	if isAlpha(code[0]) || isUnderscore(code[0]) {
		sf = scanSymbol
	}

	if sf != nil {
		return scanner(sf)
	}

	return nil
}

func selectScanner(code string) scanner {
	if s := selectFixed(code); s != nil {
		return s
	}

	return selectVaryingScanner(code)
}

func (l *eskipLex) next() (t token, err error) {
	l.code = scanWhitespace(l.code)
	if len(l.code) == 0 {
		err = eof
		return
	}

	s := selectScanner(l.code)
	if s == nil {
		err = unexpectedToken
		return
	}

	t, l.code, err = s.scan(l.code)
	if err == void {
		return l.next()
	}

	if err == nil {
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
	l.err = fmt.Errorf(
		"parse failed after token %v, position %d: %s",
		l.lastToken, l.initialLength-len(l.code), err)
}
