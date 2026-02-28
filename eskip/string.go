package eskip

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

type PrettyPrintInfo struct {
	Pretty    bool
	IndentStr string
}

func escape(s string, chars string) string {
	s = strings.ReplaceAll(s, `\`, `\\`) // escape backslash before others
	s = strings.ReplaceAll(s, "\a", `\a`)
	s = strings.ReplaceAll(s, "\b", `\b`)
	s = strings.ReplaceAll(s, "\f", `\f`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\v", `\v`)
	for i := 0; i < len(chars); i++ {
		c := chars[i : i+1]
		s = strings.ReplaceAll(s, c, `\`+c)
	}

	return s
}

func appendFmt(s []string, format string, args ...interface{}) []string {
	return append(s, fmt.Sprintf(format, args...))
}

func appendFmtEscape(s []string, format string, escapeChars string, args ...interface{}) []string {
	eargs := make([]interface{}, len(args))
	for i, arg := range args {
		eargs[i] = escape(fmt.Sprintf("%v", arg), escapeChars)
	}

	return appendFmt(s, format, eargs...)
}

func argsString(args []interface{}) string {
	var sargs []string
	for _, a := range args {
		switch v := a.(type) {
		case int:
			sargs = appendFmt(sargs, "%d", a)
		case float64:
			f := "%g"

			// imprecise elimination of 0 decimals
			// TODO: better fix this issue on parsing side
			if math.Floor(v) == v {
				f = "%.0f"
			}

			sargs = appendFmt(sargs, f, a)
		case string:
			sargs = appendFmtEscape(sargs, `"%s"`, `"`, a)
		default:
			if m, ok := a.(interface{ MarshalText() ([]byte, error) }); ok {
				t, err := m.MarshalText()
				if err != nil {
					sargs = append(sargs, `"[error]"`)
				} else {
					sargs = appendFmtEscape(sargs, `"%s"`, `"`, string(t))
				}
			} else {
				sargs = appendFmtEscape(sargs, `"%s"`, `"`, a)
			}
		}
	}

	return strings.Join(sargs, ", ")
}

func sortTail(s []string, from int) {
	if len(s)-from > 1 {
		sort.Strings(s[from:])
	}
}

func (r *Route) predicateString() string {
	var predicates []string

	if r.Path != "" {
		predicates = appendFmtEscape(predicates, `Path("%s")`, `"`, r.Path)
	}

	for _, h := range r.HostRegexps {
		predicates = appendFmtEscape(predicates, "Host(/%s/)", "/", h)
	}

	for _, p := range r.PathRegexps {
		predicates = appendFmtEscape(predicates, "PathRegexp(/%s/)", "/", p)
	}

	if r.Method != "" {
		predicates = appendFmtEscape(predicates, `Method("%s")`, `"`, r.Method)
	}

	from := len(predicates)
	for k, v := range r.Headers {
		predicates = appendFmtEscape(predicates, `Header("%s", "%s")`, `"`, k, v)
	}
	sortTail(predicates, from)

	from = len(predicates)
	for k, rxs := range r.HeaderRegexps {
		for _, rx := range rxs {
			predicates = appendFmt(predicates, `HeaderRegexp("%s", /%s/)`, escape(k, `"`), escape(rx, "/"))
		}
	}
	sortTail(predicates, from)

	for _, p := range r.Predicates {
		if p.Name != "Any" {
			predicates = appendFmt(predicates, "%s(%s)", p.Name, argsString(p.Args))
		}
	}

	if len(predicates) == 0 {
		predicates = append(predicates, "*")
	}

	return strings.Join(predicates, " && ")
}

func (r *Route) filterString(prettyPrintInfo PrettyPrintInfo) string {
	var sfilters []string
	for _, f := range r.Filters {
		sfilters = appendFmt(sfilters, "%s(%s)", f.Name, argsString(f.Args))
	}
	if prettyPrintInfo.Pretty {
		return strings.Join(sfilters, "\n"+prettyPrintInfo.IndentStr+"-> ")
	}
	return strings.Join(sfilters, " -> ")
}

func (r *Route) backendString() string {
	switch {
	case r.Shunt, r.BackendType == ShuntBackend:
		return "<shunt>"
	case r.BackendType == LoopBackend:
		return "<loopback>"
	case r.BackendType == DynamicBackend:
		return "<dynamic>"
	default:
		return r.Backend
	}
}

func lbBackendString(r *Route) string {
	var b strings.Builder
	b.WriteByte('<')
	if r.LBAlgorithm != "" {
		b.WriteString(r.LBAlgorithm)
		b.WriteString(", ")
	}
	for i, ep := range r.LBEndpoints {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('"')
		b.WriteString(ep.String())
		b.WriteByte('"')
	}
	b.WriteByte('>')
	return b.String()
}

func (r *Route) backendStringQuoted() string {
	s := r.backendString()
	switch {
	case r.BackendType == NetworkBackend && !r.Shunt:
		return fmt.Sprintf(`"%s"`, s)
	case r.BackendType == LBBackend:
		return lbBackendString(r)
	default:
		return s
	}
}

// Serializes a route expression. Omits the route id if any.
func (r *Route) String() string {
	return r.Print(PrettyPrintInfo{Pretty: false, IndentStr: ""})
}

func (r *Route) Print(prettyPrintInfo PrettyPrintInfo) string {
	s := []string{r.predicateString()}

	fs := r.filterString(prettyPrintInfo)
	if fs != "" {
		s = append(s, fs)
	}

	s = append(s, r.backendStringQuoted())
	separator := " -> "
	if prettyPrintInfo.Pretty {
		separator = "\n" + prettyPrintInfo.IndentStr + "-> "
	}
	return strings.Join(s, separator)
}

// String is the same as Print but defaulting to pretty=false.
func String(routes ...*Route) string {
	return Print(PrettyPrintInfo{Pretty: false, IndentStr: ""}, routes...)
}

// Print serializes a set of routes into a string. If there's only a
// single route, and its ID is not set, it prints only a route expression.
// If it has multiple routes with IDs, it prints full route definitions
// with the IDs and separated by ';'.
func Print(pretty PrettyPrintInfo, routes ...*Route) string {
	var buf bytes.Buffer
	Fprint(&buf, pretty, routes...)
	return buf.String()
}

func isDefinition(route *Route) bool {
	return route.Id != ""
}

func fprintExpression(w io.Writer, route *Route, prettyPrintInfo PrettyPrintInfo) {
	fmt.Fprint(w, route.Print(prettyPrintInfo))
}

func fprintDefinition(w io.Writer, route *Route, prettyPrintInfo PrettyPrintInfo) {
	fmt.Fprintf(w, "%s: %s", route.Id, route.Print(prettyPrintInfo))
}

func fprintDefinitions(w io.Writer, routes []*Route, prettyPrintInfo PrettyPrintInfo) {
	for i, r := range routes {
		if i > 0 {
			fmt.Fprint(w, "\n")
			if prettyPrintInfo.Pretty {
				fmt.Fprint(w, "\n")
			}
		}

		fprintDefinition(w, r, prettyPrintInfo)
		fmt.Fprint(w, ";")
	}
}

func Fprint(w io.Writer, prettyPrintInfo PrettyPrintInfo, routes ...*Route) {
	if len(routes) == 0 {
		return
	}

	if len(routes) == 1 && !isDefinition(routes[0]) {
		fprintExpression(w, routes[0], prettyPrintInfo)
		return
	}

	fprintDefinitions(w, routes, prettyPrintInfo)
}
