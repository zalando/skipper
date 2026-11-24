package eskip

import (
	"errors"
	"fmt"
	"strings"
)

type token struct {
	id  int
	val string
}

type charPredicate func(byte) bool

const (
	maxInternEntries   = 2048
	maxInternStringLen = 128
	routeSlabSize      = 64
	nodeSlabSize       = 64
	argsSlabSize       = 128
	ptrSlabSize        = 64
)

type eskipLex struct {
	start         int
	docMode       bool
	code          string
	lastToken     string
	lastRouteID   string
	err           error
	initialLength int
	routes        []*parsedRoute
	predicates    []*Predicate
	filters       []*Filter

	scratch       []byte
	argsBuf       []interface{}
	filtersBuf    []*Filter
	predicatesBuf []*Predicate
	stringvalsBuf []string

	strIntern      map[string]string
	anyStrIntern   map[string]interface{}
	anyFloatIntern map[float64]interface{}

	routeSlab     []parsedRoute
	filterSlab    []Filter
	predicateSlab []Predicate
	argsSlab      []interface{}
	filterPtrSlab []*Filter
	predPtrSlab   []*Predicate
	strSliceSlab  []string
}

type fixedScanner token

const (
	escapeChar  = '\\'
	decimalChar = '.'
	newlineChar = '\n'
	underscore  = '_'
)

var (
	errInvalidCharacter = errors.New("invalid character")
	errIncompleteToken  = errors.New("incomplete token")
	errUnexpectedToken  = errors.New("unexpected token")
	errVoid             = errors.New("void")
	errEOF              = errors.New("eof")
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
	{forward, "<forward>"},
}

func (fs *fixedScanner) scan(code string) (t token, rest string, err error) {
	return token(*fs), code[len(fs.val):], nil
}

func (l *eskipLex) init(start int, code string) {
	l.start = start
	l.docMode = start == start_document
	l.code = code
	l.initialLength = len(code)
}

func (l *eskipLex) reset() {
	l.start = 0
	l.docMode = false
	l.code = ""
	l.lastToken = ""
	l.lastRouteID = ""
	l.err = nil
	l.initialLength = 0
	l.routes = nil
	l.predicates = nil
	l.filters = nil

	if cap(l.scratch) > 4096 {
		l.scratch = nil
	} else {
		l.scratch = l.scratch[:0]
	}

	if cap(l.argsBuf) > 1024 {
		l.argsBuf = nil
	} else {
		clear(l.argsBuf)
		l.argsBuf = l.argsBuf[:0]
	}

	if cap(l.filtersBuf) > 1024 {
		l.filtersBuf = nil
	} else {
		clear(l.filtersBuf)
		l.filtersBuf = l.filtersBuf[:0]
	}

	if cap(l.predicatesBuf) > 1024 {
		l.predicatesBuf = nil
	} else {
		clear(l.predicatesBuf)
		l.predicatesBuf = l.predicatesBuf[:0]
	}

	if cap(l.stringvalsBuf) > 1024 {
		l.stringvalsBuf = nil
	} else {
		clear(l.stringvalsBuf)
		l.stringvalsBuf = l.stringvalsBuf[:0]
	}

	clear(l.strIntern)
	clear(l.anyStrIntern)
	clear(l.anyFloatIntern)

	l.routeSlab = nil
	l.filterSlab = nil
	l.predicateSlab = nil
	l.argsSlab = nil
	l.filterPtrSlab = nil
	l.predPtrSlab = nil
	l.strSliceSlab = nil
}

func (l *eskipLex) internString(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) > maxInternStringLen {
		return strings.Clone(s)
	}
	if l.strIntern != nil {
		if v, ok := l.strIntern[s]; ok {
			return v
		}
	}
	c := strings.Clone(s)
	if l.docMode {
		if l.strIntern == nil {
			l.strIntern = make(map[string]string, 64)
		}
		if len(l.strIntern) < maxInternEntries {
			l.strIntern[c] = c
		}
	}
	return c
}

func (l *eskipLex) internBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) > maxInternStringLen {
		return string(b)
	}
	if l.strIntern != nil {
		if v, ok := l.strIntern[string(b)]; ok {
			return v
		}
	}
	c := string(b)
	if l.docMode {
		if l.strIntern == nil {
			l.strIntern = make(map[string]string, 64)
		}
		if len(l.strIntern) < maxInternEntries {
			l.strIntern[c] = c
		}
	}
	return c
}

func (l *eskipLex) internStringAny(s string) interface{} {
	if l.docMode && len(s) <= maxInternStringLen {
		if l.anyStrIntern != nil {
			if v, ok := l.anyStrIntern[s]; ok {
				return v
			}
		}
		var v interface{} = s
		if l.anyStrIntern == nil {
			l.anyStrIntern = make(map[string]interface{}, 64)
		}
		if len(l.anyStrIntern) < maxInternEntries {
			l.anyStrIntern[s] = v
		}
		return v
	}
	return s
}

func (l *eskipLex) internFloatAny(n float64) interface{} {
	if l.docMode {
		if l.anyFloatIntern != nil {
			if v, ok := l.anyFloatIntern[n]; ok {
				return v
			}
		}
		var v interface{} = n
		if l.anyFloatIntern == nil {
			l.anyFloatIntern = make(map[float64]interface{}, 32)
		}
		if len(l.anyFloatIntern) < maxInternEntries {
			l.anyFloatIntern[n] = v
		}
		return v
	}
	return n
}

func (l *eskipLex) newParsedRoute() *parsedRoute {
	if !l.docMode {
		return &parsedRoute{}
	}
	if len(l.routeSlab) == 0 {
		l.routeSlab = make([]parsedRoute, routeSlabSize)
	}
	r := &l.routeSlab[0]
	l.routeSlab = l.routeSlab[1:]
	return r
}

func (l *eskipLex) newFilter(name string, args []interface{}) *Filter {
	if !l.docMode {
		return &Filter{Name: name, Args: args}
	}
	if len(l.filterSlab) == 0 {
		l.filterSlab = make([]Filter, nodeSlabSize)
	}
	f := &l.filterSlab[0]
	l.filterSlab = l.filterSlab[1:]
	f.Name = name
	f.Args = args
	return f
}

func (l *eskipLex) newPredicate(name string, args []interface{}) *Predicate {
	if !l.docMode {
		return &Predicate{Name: name, Args: args}
	}
	if len(l.predicateSlab) == 0 {
		l.predicateSlab = make([]Predicate, nodeSlabSize)
	}
	p := &l.predicateSlab[0]
	l.predicateSlab = l.predicateSlab[1:]
	p.Name = name
	p.Args = args
	return p
}

func (l *eskipLex) takeArgs() []interface{} {
	n := len(l.argsBuf)
	if n == 0 {
		return nil
	}
	var res []interface{}
	if l.docMode && n <= argsSlabSize {
		if len(l.argsSlab) < n {
			l.argsSlab = make([]interface{}, argsSlabSize)
		}
		res = l.argsSlab[:n:n]
		l.argsSlab = l.argsSlab[n:]
	} else {
		res = make([]interface{}, n)
	}
	copy(res, l.argsBuf)
	clear(l.argsBuf)
	l.argsBuf = l.argsBuf[:0]
	return res
}

func (l *eskipLex) takeFilters() []*Filter {
	n := len(l.filtersBuf)
	if n == 0 {
		return nil
	}
	var res []*Filter
	if l.docMode && n <= ptrSlabSize {
		if len(l.filterPtrSlab) < n {
			l.filterPtrSlab = make([]*Filter, ptrSlabSize)
		}
		res = l.filterPtrSlab[:n:n]
		l.filterPtrSlab = l.filterPtrSlab[n:]
	} else {
		res = make([]*Filter, n)
	}
	copy(res, l.filtersBuf)
	clear(l.filtersBuf)
	l.filtersBuf = l.filtersBuf[:0]
	return res
}

func (l *eskipLex) takePredicates() []*Predicate {
	n := len(l.predicatesBuf)
	if n == 0 {
		return nil
	}
	var res []*Predicate
	if l.docMode && n <= ptrSlabSize {
		if len(l.predPtrSlab) < n {
			l.predPtrSlab = make([]*Predicate, ptrSlabSize)
		}
		res = l.predPtrSlab[:n:n]
		l.predPtrSlab = l.predPtrSlab[n:]
	} else {
		res = make([]*Predicate, n)
	}
	copy(res, l.predicatesBuf)
	clear(l.predicatesBuf)
	l.predicatesBuf = l.predicatesBuf[:0]
	return res
}

func (l *eskipLex) takeStringvals() []string {
	n := len(l.stringvalsBuf)
	if n == 0 {
		return nil
	}
	var res []string
	if l.docMode && n <= ptrSlabSize {
		if len(l.strSliceSlab) < n {
			l.strSliceSlab = make([]string, ptrSlabSize)
		}
		res = l.strSliceSlab[:n:n]
		l.strSliceSlab = l.strSliceSlab[n:]
	} else {
		res = make([]string, n)
	}
	copy(res, l.stringvalsBuf)
	clear(l.stringvalsBuf)
	l.stringvalsBuf = l.stringvalsBuf[:0]
	return res
}

func isNewline(c byte) bool     { return c == newlineChar }
func isUnderscore(c byte) bool  { return c == underscore }
func isAlpha(c byte) bool       { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
func isSymbolChar(c byte) bool  { return isAlpha(c) || isDigit(c) || isUnderscore(c) }
func isDecimalChar(c byte) bool { return c == decimalChar }
func isNumberChar(c byte) bool  { return isDigit(c) || isDecimalChar(c) }

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

func (l *eskipLex) scanEscaped(delimiter byte, code string) (string, string) {
	escIdx := -1
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c == delimiter {
			return l.internString(code[:i]), code[i:]
		} else if c == escapeChar {
			escIdx = i
			break
		}
	}

	if escIdx < 0 {
		return "", ""
	}

	buf := append(l.scratch[:0], code[:escIdx]...)
	code = code[escIdx:]
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
				buf = append(buf, escapeChar)
			}

			buf = append(buf, c)
			escaped = false
		} else {
			if c == delimiter {
				l.scratch = buf
				return l.internBytes(buf), code
			}

			if c == escapeChar {
				escaped = true
			} else {
				buf = append(buf, c)
			}
		}
		code = code[1:]
	}
	l.scratch = buf
	return l.internBytes(buf), code
}

func (l *eskipLex) scanRegexp(code string) (string, string) {
	escIdx := -1
	insideGroup := false
	for i := 0; i < len(code); i++ {
		c := code[i]
		if !insideGroup && c == '[' {
			insideGroup = true
		} else if insideGroup && c == ']' {
			insideGroup = false
		} else if c == '/' && !insideGroup {
			return l.internString(code[:i]), code[i:]
		} else if c == escapeChar {
			escIdx = i
			break
		}
	}

	if escIdx < 0 {
		return "", ""
	}

	buf := append(l.scratch[:0], code[:escIdx]...)
	code = code[escIdx:]
	escaped := false
	for len(code) > 0 {
		c := code[0]
		isDelimiter := c == '/'
		isEscapeChar := c == escapeChar

		if !escaped && !insideGroup && c == '[' {
			insideGroup = true
		} else if !escaped && insideGroup && c == ']' {
			insideGroup = false
		}

		if escaped {
			if !isDelimiter && !isEscapeChar {
				buf = append(buf, escapeChar)
			}
			buf = append(buf, c)
			escaped = false
		} else {
			if isDelimiter && !insideGroup {
				l.scratch = buf
				return l.internBytes(buf), code
			}
			if isEscapeChar {
				escaped = true
			} else {
				buf = append(buf, c)
			}
		}
		code = code[1:]
	}
	l.scratch = buf
	return l.internBytes(buf), code
}

func (l *eskipLex) scanRegexpLiteral(code string) (t token, rest string, err error) {
	t.id = regexpliteral
	t.val, rest = l.scanRegexp(code[1:])
	if len(rest) == 0 {
		err = errIncompleteToken
		return
	}

	rest = rest[1:]

	return
}

func (l *eskipLex) scanRegexpOrComment(code string) (t token, rest string, err error) {
	if len(code) < 2 {
		rest = code
		err = errInvalidCharacter
		return
	}

	if code[1] == '/' {
		rest = scanComment(code)
		err = errVoid
		return
	}

	return l.scanRegexpLiteral(code)
}

func (l *eskipLex) scanStringLiteral(delimiter byte, code string) (t token, rest string, err error) {
	t.id = stringliteral
	t.val, rest = l.scanEscaped(delimiter, code[1:])
	if len(rest) == 0 {
		err = errIncompleteToken
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
func (l *eskipLex) scanDoubleQuote(code string) (token, string, error) {
	return l.scanStringLiteral('"', code)
}
func (l *eskipLex) scanBacktick(code string) (token, string, error) {
	return l.scanStringLiteral('`', code)
}

func scanNumber(code string) (t token, rest string, err error) {
	t.id = number
	decimal := false
	start := 0
	if code[0] == '-' {
		start = 1
	}

	t.val, rest = scanWhile(code[start:], func(c byte) bool {
		if isDecimalChar(c) {
			if decimal {
				return false
			}

			decimal = true
			return true
		}

		return isDigit(c)
	})
	t.val = code[:start+len(t.val)]

	if isDecimalChar(t.val[len(t.val)-1]) {
		err = errIncompleteToken
		return
	}

	return
}

func (l *eskipLex) scanSymbol(code string) (t token, rest string, err error) {
	t.id = symbol
	for i := 0; i < len(code); i++ {
		if !isSymbolChar(code[i]) {
			if code[i] == ':' {
				t.val, rest = strings.Clone(code[:i]), code[i:]
				return
			}
			t.val, rest = l.internString(code[:i]), code[i:]
			return
		}
	}
	t.val, rest = l.internString(code), ""
	return
}

func (l *eskipLex) scan(code string) (token, string, error) {
	switch code[0] {
	case ',':
		return commaToken.scan(code)
	case ')':
		return closeparenToken.scan(code)
	case '(':
		return openparenToken.scan(code)
	case ':':
		return colonToken.scan(code)
	case ';':
		return semicolonToken.scan(code)
	case '>':
		return closearrowToken.scan(code)
	case '*':
		return anyToken.scan(code)
	case '&':
		if len(code) >= 2 && code[1] == '&' {
			return andToken.scan(code)
		}
	case '-':
		if len(code) >= 2 && code[1] == '>' {
			return arrowToken.scan(code)
		}
		if len(code) >= 2 && isNumberChar(code[1]) {
			return scanNumber(code)
		}
	case '/':
		return l.scanRegexpOrComment(code)
	case '"':
		return l.scanDoubleQuote(code)
	case '`':
		return l.scanBacktick(code)
	case '<':
		for _, tok := range openarrowPrefixedTokens {
			if strings.HasPrefix(code, tok.val) {
				return tok.scan(code)
			}
		}
		return openarrowToken.scan(code)
	}

	if isNumberChar(code[0]) {
		return scanNumber(code)
	}

	if isAlpha(code[0]) || isUnderscore(code[0]) {
		return l.scanSymbol(code)
	}

	return token{}, "", errUnexpectedToken
}

func (l *eskipLex) next() (token, error) {
	l.code = scanWhitespace(l.code)
	if len(l.code) == 0 {
		return token{}, errEOF
	}

	t, rest, err := l.scan(l.code)
	if err == errUnexpectedToken {
		return token{}, err
	}
	l.code = rest

	if err == errVoid {
		return l.next()
	}

	if err == nil {
		l.lastToken = t.val
	}

	return t, err
}

func (l *eskipLex) Lex(lval *eskipSymType) int {
	// first emit the start token
	if l.start != 0 {
		start := l.start
		l.start = 0
		return start
	}

	t, err := l.next()
	if err == errEOF {
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
	lastRouteID := ""
	if l.lastRouteID != "" {
		lastRouteID = ", last route id: " + l.lastRouteID
	}
	l.err = fmt.Errorf(
		"parse failed after token %s%s, position %d: %s",
		l.lastToken, lastRouteID, l.initialLength-len(l.code), err)
}
