//line parser.y:16
package eskip

import __yyfmt__ "fmt"

//line parser.y:16
import "strconv"

// conversion error ignored, tokenizer expression already checked format
func convertNumber(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

//line parser.y:28
type eskipSymType struct {
	yys       int
	token     string
	route     *parsedRoute
	routes    []*parsedRoute
	matchers  []*matcher
	matcher   *matcher
	filter    *Filter
	filters   []*Filter
	args      []interface{}
	arg       interface{}
	backend   string
	shunt     bool
	loopback  bool
	numval    float64
	stringval string
	regexpval string
}

const and = 57346
const any = 57347
const arrow = 57348
const closeparen = 57349
const colon = 57350
const comma = 57351
const number = 57352
const openparen = 57353
const regexpliteral = 57354
const semicolon = 57355
const shunt = 57356
const loopback = 57357
const stringliteral = 57358
const symbol = 57359

var eskipToknames = [...]string{
	"$end",
	"error",
	"$unk",
	"and",
	"any",
	"arrow",
	"closeparen",
	"colon",
	"comma",
	"number",
	"openparen",
	"regexpliteral",
	"semicolon",
	"shunt",
	"loopback",
	"stringliteral",
	"symbol",
}
var eskipStatenames = [...]string{}

const eskipEofCode = 1
const eskipErrCode = 2
const eskipInitialStackSize = 16

//line parser.y:212

//line yacctab:1
var eskipExca = [...]int{
	-1, 1,
	1, -1,
	-2, 0,
}

const eskipPrivate = 57344

const eskipLast = 47

var eskipAct = [...]int{

	29, 31, 22, 28, 16, 17, 20, 21, 23, 24,
	33, 9, 34, 19, 9, 3, 23, 10, 7, 14,
	13, 36, 4, 26, 32, 43, 8, 38, 37, 27,
	38, 25, 12, 15, 11, 35, 30, 19, 40, 42,
	41, 39, 18, 5, 6, 2, 1,
}
var eskipPact = [...]int{

	9, -1000, 4, -1000, -1000, 28, 12, -1000, 8, -1000,
	-13, -8, 6, 6, 0, -1000, -1000, -1000, 29, -1000,
	-1000, -1000, -1000, -1000, 10, -1000, 8, -1000, 21, -1000,
	-1000, -1000, -1000, -1000, -1000, -8, 0, -1000, 0, -1000,
	-1000, 18, -1000, -1000,
}
var eskipPgo = [...]int{

	0, 46, 45, 15, 22, 44, 43, 5, 42, 18,
	3, 2, 0, 36, 1, 24,
}
var eskipR1 = [...]int{

	0, 1, 1, 2, 2, 2, 2, 4, 5, 3,
	3, 6, 6, 9, 9, 8, 8, 11, 10, 10,
	10, 12, 12, 12, 7, 7, 7, 13, 14, 15,
}
var eskipR2 = [...]int{

	0, 1, 1, 0, 1, 3, 2, 3, 1, 3,
	5, 1, 3, 1, 4, 1, 3, 4, 0, 1,
	3, 1, 1, 1, 1, 1, 1, 1, 1, 1,
}
var eskipChk = [...]int{

	-1000, -1, -2, -3, -4, -6, -5, -9, 17, 5,
	13, 6, 4, 8, 11, -4, 17, -7, -8, -14,
	14, 15, -11, 16, 17, -9, 17, -3, -10, -12,
	-13, -14, -15, 10, 12, 6, 11, 7, 9, -7,
	-11, -10, -12, 7,
}
var eskipDef = [...]int{

	3, -2, 1, 2, 4, 0, 0, 11, 8, 13,
	6, 0, 0, 0, 18, 5, 8, 9, 0, 24,
	25, 26, 15, 28, 0, 12, 0, 7, 0, 19,
	21, 22, 23, 27, 29, 0, 18, 14, 0, 10,
	16, 0, 20, 17,
}
var eskipTok1 = [...]int{

	1,
}
var eskipTok2 = [...]int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17,
}
var eskipTok3 = [...]int{
	0,
}

var eskipErrorMessages = [...]struct {
	state int
	token int
	msg   string
}{}

//line yaccpar:1

/*	parser for yacc output	*/

var (
	eskipDebug        = 0
	eskipErrorVerbose = false
)

type eskipLexer interface {
	Lex(lval *eskipSymType) int
	Error(s string)
}

type eskipParser interface {
	Parse(eskipLexer) int
	Lookahead() int
}

type eskipParserImpl struct {
	lval  eskipSymType
	stack [eskipInitialStackSize]eskipSymType
	char  int
}

func (eskiprcvr *eskipParserImpl) Lookahead() int {
	return eskiprcvr.char
}

func eskipNewParser() eskipParser {
	return &eskipParserImpl{}
}

const eskipFlag = -1000

func eskipTokname(c int) string {
	if c >= 1 && c-1 < len(eskipToknames) {
		if eskipToknames[c-1] != "" {
			return eskipToknames[c-1]
		}
	}
	return __yyfmt__.Sprintf("tok-%v", c)
}

func eskipStatname(s int) string {
	if s >= 0 && s < len(eskipStatenames) {
		if eskipStatenames[s] != "" {
			return eskipStatenames[s]
		}
	}
	return __yyfmt__.Sprintf("state-%v", s)
}

func eskipErrorMessage(state, lookAhead int) string {
	const TOKSTART = 4

	if !eskipErrorVerbose {
		return "syntax error"
	}

	for _, e := range eskipErrorMessages {
		if e.state == state && e.token == lookAhead {
			return "syntax error: " + e.msg
		}
	}

	res := "syntax error: unexpected " + eskipTokname(lookAhead)

	// To match Bison, suggest at most four expected tokens.
	expected := make([]int, 0, 4)

	// Look for shiftable tokens.
	base := eskipPact[state]
	for tok := TOKSTART; tok-1 < len(eskipToknames); tok++ {
		if n := base + tok; n >= 0 && n < eskipLast && eskipChk[eskipAct[n]] == tok {
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}
	}

	if eskipDef[state] == -2 {
		i := 0
		for eskipExca[i] != -1 || eskipExca[i+1] != state {
			i += 2
		}

		// Look for tokens that we accept or reduce.
		for i += 2; eskipExca[i] >= 0; i += 2 {
			tok := eskipExca[i]
			if tok < TOKSTART || eskipExca[i+1] == 0 {
				continue
			}
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}

		// If the default action is to accept or reduce, give up.
		if eskipExca[i+1] != 0 {
			return res
		}
	}

	for i, tok := range expected {
		if i == 0 {
			res += ", expecting "
		} else {
			res += " or "
		}
		res += eskipTokname(tok)
	}
	return res
}

func eskiplex1(lex eskipLexer, lval *eskipSymType) (char, token int) {
	token = 0
	char = lex.Lex(lval)
	if char <= 0 {
		token = eskipTok1[0]
		goto out
	}
	if char < len(eskipTok1) {
		token = eskipTok1[char]
		goto out
	}
	if char >= eskipPrivate {
		if char < eskipPrivate+len(eskipTok2) {
			token = eskipTok2[char-eskipPrivate]
			goto out
		}
	}
	for i := 0; i < len(eskipTok3); i += 2 {
		token = eskipTok3[i+0]
		if token == char {
			token = eskipTok3[i+1]
			goto out
		}
	}

out:
	if token == 0 {
		token = eskipTok2[1] /* unknown char */
	}
	if eskipDebug >= 3 {
		__yyfmt__.Printf("lex %s(%d)\n", eskipTokname(token), uint(char))
	}
	return char, token
}

func eskipParse(eskiplex eskipLexer) int {
	return eskipNewParser().Parse(eskiplex)
}

func (eskiprcvr *eskipParserImpl) Parse(eskiplex eskipLexer) int {
	var eskipn int
	var eskipVAL eskipSymType
	var eskipDollar []eskipSymType
	_ = eskipDollar // silence set and not used
	eskipS := eskiprcvr.stack[:]

	Nerrs := 0   /* number of errors */
	Errflag := 0 /* error recovery flag */
	eskipstate := 0
	eskiprcvr.char = -1
	eskiptoken := -1 // eskiprcvr.char translated into internal numbering
	defer func() {
		// Make sure we report no lookahead when not parsing.
		eskipstate = -1
		eskiprcvr.char = -1
		eskiptoken = -1
	}()
	eskipp := -1
	goto eskipstack

ret0:
	return 0

ret1:
	return 1

eskipstack:
	/* put a state and value onto the stack */
	if eskipDebug >= 4 {
		__yyfmt__.Printf("char %v in %v\n", eskipTokname(eskiptoken), eskipStatname(eskipstate))
	}

	eskipp++
	if eskipp >= len(eskipS) {
		nyys := make([]eskipSymType, len(eskipS)*2)
		copy(nyys, eskipS)
		eskipS = nyys
	}
	eskipS[eskipp] = eskipVAL
	eskipS[eskipp].yys = eskipstate

eskipnewstate:
	eskipn = eskipPact[eskipstate]
	if eskipn <= eskipFlag {
		goto eskipdefault /* simple state */
	}
	if eskiprcvr.char < 0 {
		eskiprcvr.char, eskiptoken = eskiplex1(eskiplex, &eskiprcvr.lval)
	}
	eskipn += eskiptoken
	if eskipn < 0 || eskipn >= eskipLast {
		goto eskipdefault
	}
	eskipn = eskipAct[eskipn]
	if eskipChk[eskipn] == eskiptoken { /* valid shift */
		eskiprcvr.char = -1
		eskiptoken = -1
		eskipVAL = eskiprcvr.lval
		eskipstate = eskipn
		if Errflag > 0 {
			Errflag--
		}
		goto eskipstack
	}

eskipdefault:
	/* default state action */
	eskipn = eskipDef[eskipstate]
	if eskipn == -2 {
		if eskiprcvr.char < 0 {
			eskiprcvr.char, eskiptoken = eskiplex1(eskiplex, &eskiprcvr.lval)
		}

		/* look through exception table */
		xi := 0
		for {
			if eskipExca[xi+0] == -1 && eskipExca[xi+1] == eskipstate {
				break
			}
			xi += 2
		}
		for xi += 2; ; xi += 2 {
			eskipn = eskipExca[xi+0]
			if eskipn < 0 || eskipn == eskiptoken {
				break
			}
		}
		eskipn = eskipExca[xi+1]
		if eskipn < 0 {
			goto ret0
		}
	}
	if eskipn == 0 {
		/* error ... attempt to resume parsing */
		switch Errflag {
		case 0: /* brand new error */
			eskiplex.Error(eskipErrorMessage(eskipstate, eskiptoken))
			Nerrs++
			if eskipDebug >= 1 {
				__yyfmt__.Printf("%s", eskipStatname(eskipstate))
				__yyfmt__.Printf(" saw %s\n", eskipTokname(eskiptoken))
			}
			fallthrough

		case 1, 2: /* incompletely recovered error ... try again */
			Errflag = 3

			/* find a state where "error" is a legal shift action */
			for eskipp >= 0 {
				eskipn = eskipPact[eskipS[eskipp].yys] + eskipErrCode
				if eskipn >= 0 && eskipn < eskipLast {
					eskipstate = eskipAct[eskipn] /* simulate a shift of "error" */
					if eskipChk[eskipstate] == eskipErrCode {
						goto eskipstack
					}
				}

				/* the current p has no shift on "error", pop stack */
				if eskipDebug >= 2 {
					__yyfmt__.Printf("error recovery pops state %d\n", eskipS[eskipp].yys)
				}
				eskipp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if eskipDebug >= 2 {
				__yyfmt__.Printf("error recovery discards %s\n", eskipTokname(eskiptoken))
			}
			if eskiptoken == eskipEofCode {
				goto ret1
			}
			eskiprcvr.char = -1
			eskiptoken = -1
			goto eskipnewstate /* try again in the same state */
		}
	}

	/* reduction by production eskipn */
	if eskipDebug >= 2 {
		__yyfmt__.Printf("reduce %v in:\n\t%v\n", eskipn, eskipStatname(eskipstate))
	}

	eskipnt := eskipn
	eskippt := eskipp
	_ = eskippt // guard against "declared and not used"

	eskipp -= eskipR2[eskipn]
	// eskipp is now the index of $0. Perform the default action. Iff the
	// reduced production is ε, $1 is possibly out of range.
	if eskipp+1 >= len(eskipS) {
		nyys := make([]eskipSymType, len(eskipS)*2)
		copy(nyys, eskipS)
		eskipS = nyys
	}
	eskipVAL = eskipS[eskipp+1]

	/* consult goto table to find next state */
	eskipn = eskipR1[eskipn]
	eskipg := eskipPgo[eskipn]
	eskipj := eskipg + eskipS[eskipp].yys + 1

	if eskipj >= eskipLast {
		eskipstate = eskipAct[eskipg]
	} else {
		eskipstate = eskipAct[eskipj]
		if eskipChk[eskipstate] != -eskipn {
			eskipstate = eskipAct[eskipg]
		}
	}
	// dummy call; replaced with literal code
	switch eskipnt {

	case 1:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:64
		{
			eskipVAL.routes = eskipDollar[1].routes
			eskiplex.(*eskipLex).routes = eskipVAL.routes
		}
	case 2:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:69
		{
			eskipVAL.routes = []*parsedRoute{eskipDollar[1].route}
			eskiplex.(*eskipLex).routes = eskipVAL.routes
		}
	case 4:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:76
		{
			eskipVAL.routes = []*parsedRoute{eskipDollar[1].route}
		}
	case 5:
		eskipDollar = eskipS[eskippt-3 : eskippt+1]
		//line parser.y:80
		{
			eskipVAL.routes = eskipDollar[1].routes
			eskipVAL.routes = append(eskipVAL.routes, eskipDollar[3].route)
		}
	case 6:
		eskipDollar = eskipS[eskippt-2 : eskippt+1]
		//line parser.y:85
		{
			eskipVAL.routes = eskipDollar[1].routes
		}
	case 7:
		eskipDollar = eskipS[eskippt-3 : eskippt+1]
		//line parser.y:90
		{
			eskipVAL.route = eskipDollar[3].route
			eskipVAL.route.id = eskipDollar[1].token
		}
	case 8:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:96
		{
			eskipVAL.token = eskipDollar[1].token
		}
	case 9:
		eskipDollar = eskipS[eskippt-3 : eskippt+1]
		//line parser.y:101
		{
			eskipVAL.route = &parsedRoute{
				matchers: eskipDollar[1].matchers,
				backend:  eskipDollar[3].backend,
				shunt:    eskipDollar[3].shunt,
				loopback: eskipDollar[3].loopback}
		}
	case 10:
		eskipDollar = eskipS[eskippt-5 : eskippt+1]
		//line parser.y:109
		{
			eskipVAL.route = &parsedRoute{
				matchers: eskipDollar[1].matchers,
				filters:  eskipDollar[3].filters,
				backend:  eskipDollar[5].backend,
				shunt:    eskipDollar[5].shunt,
				loopback: eskipDollar[5].loopback}
			eskipDollar[1].matchers = nil
			eskipDollar[3].filters = nil
		}
	case 11:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:121
		{
			eskipVAL.matchers = []*matcher{eskipDollar[1].matcher}
		}
	case 12:
		eskipDollar = eskipS[eskippt-3 : eskippt+1]
		//line parser.y:125
		{
			eskipVAL.matchers = eskipDollar[1].matchers
			eskipVAL.matchers = append(eskipVAL.matchers, eskipDollar[3].matcher)
		}
	case 13:
		_ = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:131
		{
			eskipVAL.matcher = &matcher{"*", nil}
		}
	case 14:
		eskipDollar = eskipS[eskippt-4 : eskippt+1]
		//line parser.y:135
		{
			eskipVAL.matcher = &matcher{eskipDollar[1].token, eskipDollar[3].args}
			eskipDollar[3].args = nil
		}
	case 15:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:141
		{
			eskipVAL.filters = []*Filter{eskipDollar[1].filter}
		}
	case 16:
		eskipDollar = eskipS[eskippt-3 : eskippt+1]
		//line parser.y:145
		{
			eskipVAL.filters = eskipDollar[1].filters
			eskipVAL.filters = append(eskipVAL.filters, eskipDollar[3].filter)
		}
	case 17:
		eskipDollar = eskipS[eskippt-4 : eskippt+1]
		//line parser.y:151
		{
			eskipVAL.filter = &Filter{
				Name: eskipDollar[1].token,
				Args: eskipDollar[3].args}
			eskipDollar[3].args = nil
		}
	case 19:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:160
		{
			eskipVAL.args = []interface{}{eskipDollar[1].arg}
		}
	case 20:
		eskipDollar = eskipS[eskippt-3 : eskippt+1]
		//line parser.y:164
		{
			eskipVAL.args = eskipDollar[1].args
			eskipVAL.args = append(eskipVAL.args, eskipDollar[3].arg)
		}
	case 21:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:170
		{
			eskipVAL.arg = eskipDollar[1].numval
		}
	case 22:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:174
		{
			eskipVAL.arg = eskipDollar[1].stringval
		}
	case 23:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:178
		{
			eskipVAL.arg = eskipDollar[1].regexpval
		}
	case 24:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:183
		{
			eskipVAL.backend = eskipDollar[1].stringval
			eskipVAL.shunt = false
			eskipVAL.loopback = false
		}
	case 25:
		_ = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:189
		{
			eskipVAL.shunt = true
		}
	case 26:
		_ = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:193
		{
			eskipVAL.loopback = true
		}
	case 27:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:198
		{
			eskipVAL.numval = convertNumber(eskipDollar[1].token)
		}
	case 28:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:203
		{
			eskipVAL.stringval = eskipDollar[1].token
		}
	case 29:
		eskipDollar = eskipS[eskippt-1 : eskippt+1]
		//line parser.y:208
		{
			eskipVAL.regexpval = eskipDollar[1].token
		}
	}
	goto eskipstack /* stack new state and value */
}
