package eskip

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strings"
)

type PrettyPrintInfo struct {
	Pretty    bool
	IndentStr string
}

func escape(s string, chars string) string {

	s = strings.Replace(s, "\\", "\\\\", -1)
	for i := 0; i < len(chars); i++ {
		c := chars[i : i+1]
		s = strings.Replace(s, c, "\\"+c, -1)
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
		}
	}

	return strings.Join(sargs, ", ")
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

	for k, v := range r.Headers {
		predicates = appendFmtEscape(predicates, `Header("%s", "%s")`, `"`, k, v)
	}

	for k, rxs := range r.HeaderRegexps {
		for _, rx := range rxs {
			predicates = appendFmt(predicates, `HeaderRegexp("%s", /%s/)`, escape(k, `"`), escape(rx, "/"))
		}
	}

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
	default:
		return r.Backend
	}
}

func (r *Route) backendStringQuoted() string {
	s := r.backendString()
	if r.BackendType == NetworkBackend && !r.Shunt {
		s = fmt.Sprintf(`"%s"`, s)
	}

	return s
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
