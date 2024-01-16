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
	lastToken     string
	lastRouteID   string
	err           error
	initialLength int
	routes        []*parsedRoute
}

type fixedScanner token

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

var (
	andToken        = &fixedScanner{and, "&&"}
	anyToken        = &fixedScanner{any, "*"}
	arrowToken      = &fixedScanner{arrow, "->"}
	closeparenToken = &fixedScanner{closeparen, ")"}
	colonToken      = &fixedScanner{colon, ":"}
	commaToken      = &fixedScanner{comma, ","}
	openparenToken  = &fixedScanner{openparen, "("}
	semicolonToken  = &fixedScanner{semicolon, ";"}
	openarrowToken  = &fixedScanner{openarrow, "<"}
	closearrowToken = &fixedScanner{closearrow, ">"}
)

var openarrowPrefixedTokens = []*fixedScanner{
	{shunt, "<shunt>"},
	{loopback, "<loopback>"},
	{dynamic, "<dynamic>"},
}

func (fs *fixedScanner) scan(code string) (t token, rest string, err error) {
	return token(*fs), code[len(fs.val):], nil
}

func (l *eskipLex) init(code string) {
	l.code = code
	l.initialLength = len(code)
}

func isNewline(c byte) bool     { return c == newlineChar }
func isUnderscore(c byte) bool  { return c == underscore }
func isAlpha(c byte) bool       { return unicode.IsLetter(rune(c)) }
func isDigit(c byte) bool       { return unicode.IsDigit(rune(c)) }
func isSymbolChar(c byte) bool  { return isUnderscore(c) || isAlpha(c) || isDigit(c) }
func isDecimalChar(c byte) bool { return c == decimalChar }
func isNumberChar(c byte) bool  { return isDecimalChar(c) || isDigit(c) }

func scanWhile(code string, p charPredicate) (string, string) {
	for i := 0; i < len(code); i++ {
		if !p(code[i]) {
			return code[0:i], code[i:]
		}
	}
	return code, ""
}

func scanVoid(code string, p charPredicate) string {
	_, rest := scanWhile(code, p)
	return rest
}

func scanEscaped(delimiter byte, code string) (string, string) {
	// fast path: check if there is a delimiter without preceding escape character
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c == delimiter {
			return code[:i], code[i:]
		} else if c == escapeChar {
			break
		}
	}

	var sb strings.Builder
	escaped := false
	for len(code) > 0 {
		c := code[0]

		if escaped {
			switch c {
			case 'a':
				c = '\a'
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case 'v':
				c = '\v'
			case delimiter:
			case escapeChar:
			default:
				sb.WriteByte(escapeChar)
			}

			sb.WriteByte(c)
			escaped = false
		} else {
			if c == delimiter {
				return sb.String(), code
			}

			if c == escapeChar {
				escaped = true
			} else {
				sb.WriteByte(c)
			}
		}
		code = code[1:]
	}
	return sb.String(), code
}

func scanRegexp(code string) (string, string) {
	var sb strings.Builder
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
			//delimiter / is escaped in PathRegexp so that it means no end PathRegexp(/\//)
			if !isDelimiter && !isEscapeChar {
				sb.WriteByte(escapeChar)
			}
			sb.WriteByte(c)
			escaped = false
		} else {
			if isDelimiter && !insideGroup {
				return sb.String(), code
			}
			if isEscapeChar {
				escaped = true
			} else {
				sb.WriteByte(c)
			}
		}
		code = code[1:]
	}
	return sb.String(), code
}

func scanRegexpLiteral(code string) (t token, rest string, err error) {
	t.id = regexpliteral
	t.val, rest = scanRegexp(code[1:])
	if len(rest) == 0 {
		err = incompleteToken
		return
	}

	rest = rest[1:]

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

	return scanRegexpLiteral(code)
}

func scanStringLiteral(delimiter byte, code string) (t token, rest string, err error) {
	t.id = stringliteral
	t.val, rest = scanEscaped(delimiter, code[1:])
	if len(rest) == 0 {
		err = incompleteToken
		return
	}

	rest = rest[1:]

	return
}

func scanWhitespace(code string) string {
	start := 0
	for ; start < len(code); start++ {
		c := code[start]
		// check frequent values first
		if c != ' ' && c != '\n' && c != '\t' && c != '\v' && c != '\f' && c != '\r' && c != 0x85 && c != 0xA0 {
			break
		}
	}
	return code[start:]
}
func scanComment(code string) string {
	return scanVoid(code, func(c byte) bool { return !isNewline(c) })
}
func scanDoubleQuote(code string) (token, string, error) { return scanStringLiteral('"', code) }
func scanBacktick(code string) (token, string, error)    { return scanStringLiteral('`', code) }

func scanNumber(code string) (t token, rest string, err error) {
	t.id = number
	decimal := false
	t.val, rest = scanWhile(code, func(c byte) bool {
		if isDecimalChar(c) {
			if decimal {
				return false
			}

			decimal = true
			return true
		}

		return isDigit(c)
	})

	if isDecimalChar(t.val[len(t.val)-1]) {
		err = incompleteToken
		return
	}

	return
}

func scanSymbol(code string) (t token, rest string, err error) {
	t.id = symbol
	t.val, rest = scanWhile(code, isSymbolChar)
	return
}

func selectScanner(code string) scanner {
	switch code[0] {
	case ',':
		return commaToken
	case ')':
		return closeparenToken
	case '(':
		return openparenToken
	case ':':
		return colonToken
	case ';':
		return semicolonToken
	case '>':
		return closearrowToken
	case '*':
		return anyToken
	case '&':
		if len(code) >= 2 && code[1] == '&' {
			return andToken
		}
	case '-':
		if len(code) >= 2 && code[1] == '>' {
			return arrowToken
		}
	case '/':
		return scannerFunc(scanRegexpOrComment)
	case '"':
		return scannerFunc(scanDoubleQuote)
	case '`':
		return scannerFunc(scanBacktick)
	case '<':
		for _, tok := range openarrowPrefixedTokens {
			if strings.HasPrefix(code, tok.val) {
				return tok
			}
		}
		return openarrowToken
	}

	if isNumberChar(code[0]) {
		return scannerFunc(scanNumber)
	}

	if isAlpha(code[0]) || isUnderscore(code[0]) {
		return scannerFunc(scanSymbol)
	}

	return nil
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
		l.lastToken = t.val
	}

	return
}

func (l *eskipLex) Lex(lval *eskipSymType) int {
	t, err := l.next()
	if err == eof {
		return -1
	}

	if err != nil {
		l.Error(err.Error())
		return -1
	}

	lval.token = t.val
	return t.id
}

func (l *eskipLex) Error(err string) {
	l.err = fmt.Errorf(
		"parse failed after token %s, last route id: %v, position %d: %s",
		l.lastToken, l.lastRouteID, l.initialLength-len(l.code), err)
}
