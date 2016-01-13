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

%{
package eskip

import "strconv"

// conversion error ignored, tokenizer expression already checked format
func convertNumber(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

%}

%union {
	token string
	route *parsedRoute
	routes []*parsedRoute
	matchers []*matcher
	matcher *matcher
	filter *Filter
	filters []*Filter
	args []interface{}
	arg interface{}
	backend string
	shunt bool
	numval float64
	stringval string
	regexpval string
}

%token and
%token any
%token arrow
%token closeparen
%token colon
%token comma
%token number
%token openparen
%token regexpliteral
%token semicolon
%token shunt
%token stringliteral
%token symbol

%%

document:
	routes {
		$$.routes = $1.routes
		eskiplex.(*eskipLex).routes = $$.routes
	}
	|
	route {
		$$.routes = []*parsedRoute{$1.route}
		eskiplex.(*eskipLex).routes = $$.routes
	}

routes:
	|
	routedef {
		$$.routes = []*parsedRoute{$1.route}
	}
	|
	routes semicolon routedef {
		$$.routes = $1.routes
		$$.routes = append($$.routes, $3.route)
	}
	|
	routes semicolon {
		$$.routes = $1.routes
	}

routedef:
	routeid colon route {
		$$.route = $3.route
		$$.route.id = $1.token
	}

routeid:
	symbol {
		$$.token = $1.token
	}

route:
	frontend arrow backend {
		$$.route = &parsedRoute{
			matchers: $1.matchers,
			backend: $3.backend,
			shunt: $3.shunt}
	}
	|
	frontend arrow filters arrow backend {
		$$.route = &parsedRoute{
			matchers: $1.matchers,
			filters: $3.filters,
			backend: $5.backend,
			shunt: $5.shunt}
		$1.matchers = nil
		$3.filters = nil
	}

frontend:
	matcher {
		$$.matchers = []*matcher{$1.matcher}
	}
	|
	frontend and matcher {
		$$.matchers = $1.matchers
		$$.matchers = append($$.matchers, $3.matcher)
	}

matcher:
    any {
        $$.matcher = &matcher{"*", nil}
    }
    |
	symbol openparen args closeparen {
        $$.matcher = &matcher{$1.token, $3.args}
		$3.args = nil
	}

filters:
	filter {
		$$.filters = []*Filter{$1.filter}
	}
	|
	filters arrow filter {
		$$.filters = $1.filters
		$$.filters = append($$.filters, $3.filter)
	}

filter:
	symbol openparen args closeparen {
		$$.filter = &Filter{
			Name: $1.token,
			Args: $3.args}
		$3.args = nil
	}

args:
	|
	arg {
		$$.args = []interface{}{$1.arg}
	}
	|
	args comma arg {
		$$.args = $1.args
		$$.args = append($$.args, $3.arg)
	}

arg:
	numval {
		$$.arg = $1.numval
	}
	|
	stringval {
		$$.arg = $1.stringval
	}
	|
	regexpval {
		$$.arg = $1.regexpval
	}

backend:
	stringval {
		$$.backend = $1.stringval
		$$.shunt = false
	}
	|
	shunt {
		$$.shunt = true
	}

numval:
	number {
		$$.numval = convertNumber($1.token)
	}

stringval:
	stringliteral {
		$$.stringval = $1.token
	}

regexpval:
	regexpliteral {
		$$.regexpval = $1.token
	}

%%
