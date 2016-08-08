package eskip

import (
	"fmt"
	"strings"
)

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
		switch a.(type) {
		case float64:
			sargs = appendFmt(sargs, "%g", a)
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

func (r *Route) filterString(pretty bool) string {
	var sfilters []string
	for _, f := range r.Filters {
		sfilters = appendFmt(sfilters, "%s(%s)", f.Name, argsString(f.Args))
	}
	if pretty {
		return strings.Join(sfilters, "\n  -> ")
	}
	return strings.Join(sfilters, " -> ")
}

func (r *Route) backendString() string {
	if r.Shunt {
		return "<shunt>"
	}

	return fmt.Sprintf(`"%s"`, r.Backend)
}

// Serializes a route expression. Omits the route id if any.
func (r *Route) String() string {
	return r.Print(false)
}

func (r *Route) Print(pretty bool) string {
	s := []string{r.predicateString()}

	fs := r.filterString(pretty)
	if fs != "" {
		s = append(s, fs)
	}

	s = append(s, r.backendString())
	separator := " -> "
	if pretty {
		separator = "\n  -> "
	}
	return strings.Join(s, separator)
}

// Serializes a set of routes.
func String(routes ...*Route) string {
	return Print(false, routes...)
}

func Print(pretty bool, routes ...*Route) string {
	if len(routes) == 1 && routes[0].Id == "" {
		return routes[0].Print(pretty)
	}

	rs := make([]string, len(routes))
	for i, r := range routes {
		rs[i] = fmt.Sprintf("%s: %s", r.Id, r.Print(pretty))
	}

	return strings.Join(rs, ";\n")
}
