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

func (r *Route) condString() string {
	var conds []string

	if r.Path != "" {
		conds = appendFmtEscape(conds, `Path("%s")`, `"`, r.Path)
	}

	for _, h := range r.HostRegexps {
		conds = appendFmtEscape(conds, "Host(/%s/)", "/", h)
	}

	for _, p := range r.PathRegexps {
		conds = appendFmtEscape(conds, "PathRegexp(/%s/)", "/", p)
	}

	if r.Method != "" {
		conds = appendFmtEscape(conds, `Method("%s")`, `"`, r.Method)
	}

	for k, v := range r.Headers {
		conds = appendFmtEscape(conds, `Header("%s", "%s")`, `"`, k, v)
	}

	for k, rxs := range r.HeaderRegexps {
		for _, rx := range rxs {
			conds = appendFmt(conds, `HeaderRegexp("%s", /%s/)`, escape(k, `"`), escape(rx, "/"))
		}
	}

	if len(conds) == 0 {
		conds = append(conds, "Any()")
	}

	return strings.Join(conds, " && ")
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

func (r *Route) filterString() string {
	var sfilters []string
	for _, f := range r.Filters {
		sfilters = appendFmt(sfilters, "%s(%s)", f.Name, argsString(f.Args))
	}

	return strings.Join(sfilters, " -> ")
}

func (r *Route) backendString() string {
	if r.Shunt {
		return "<shunt>"
	}

	return fmt.Sprintf(`"%s"`, r.Backend)
}

func (r *Route) String() string {
	s := []string{r.condString()}

	fs := r.filterString()
	if fs != "" {
		s = append(s, fs)
	}

	s = append(s, r.backendString())
	return strings.Join(s, " -> ")
}

func String(routes ...*Route) string {
	rs := make([]string, len(routes))
	for i, r := range routes {
		rs[i] = fmt.Sprintf("%s: %s", r.Id, r.String())
	}

	return strings.Join(rs, ";\n")
}
