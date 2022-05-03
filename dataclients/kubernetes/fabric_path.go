package kubernetes

import (
	"regexp"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
)

var (
	slash        = `/`
	star         = `\*`
	curlyOpen    = `{`
	curlyClose   = `}`
	end          = `$`
	wildcardName = `(?P<name>[\w-]+?)`
	repls        = []struct {
		match *regexp.Regexp
		repl  string
	}{
		{regexp.MustCompile(slash + star + wildcardName + slash), `/:$name/`},
		{regexp.MustCompile(slash + star + wildcardName + end), `/:$name`},
		{regexp.MustCompile(curlyOpen + wildcardName + curlyClose), `:$name`},
		{regexp.MustCompile(slash + star + slash), `/:id/`},
		{regexp.MustCompile(slash + star + end), `/:id`},
	}
)

// fabricPathStrToPredicate takes a Fabric path string and transforms it an equivalent Skipper Path* predicate
func fabricPathStrToPredicate(fps string) *eskip.Predicate {
	if fps == "/**" {
		return &eskip.Predicate{Name: predicates.PathSubtreeName, Args: []interface{}{"/"}}
	}

	for _, repl := range repls {
		fps = repl.match.ReplaceAllString(fps, repl.repl)
	}

	return &eskip.Predicate{
		Name: predicates.PathName,
		Args: []interface{}{fps},
	}
}

func applyPath(r *eskip.Route, fp *definitions.FabricPath) {
	r.Predicates = append(r.Predicates, fabricPathStrToPredicate(fp.Path))
}
