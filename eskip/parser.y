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
//lint:file-ignore ST1016 This is a generated file.
//lint:file-ignore SA4006 This is a generated file.

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
	loopback bool
	dynamic bool
	lbBackend bool
	numval float64
	stringval string
	regexpval string
	stringvals []string
	lbAlgorithm string
	lbEndpoints []string
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
%token loopback
%token dynamic
%token stringliteral
%token symbol
%token openarrow
%token closearrow

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
			shunt: $3.shunt,
			loopback: $3.loopback,
			dynamic: $3.dynamic,
			lbBackend: $3.lbBackend,
			lbAlgorithm: $3.lbAlgorithm,
			lbEndpoints: $3.lbEndpoints,
		}
	}
	|
	frontend arrow filters arrow backend {
		$$.route = &parsedRoute{
			matchers: $1.matchers,
			filters: $3.filters,
			backend: $5.backend,
			shunt: $5.shunt,
			loopback: $5.loopback,
			dynamic: $5.dynamic,
			lbBackend: $3.lbBackend,
			lbAlgorithm: $3.lbAlgorithm,
			lbEndpoints: $3.lbEndpoints,
		}
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

stringvals:
	stringval {
		$$.stringvals = []string{$1.stringval}
	}
	|
	stringvals comma stringval {
		$$.stringvals = $1.stringvals
		$$.stringvals = append($$.stringvals, $3.stringval)
	}

lbbackendbody:
	stringvals {
		$$.lbEndpoints = $1.stringvals
	}
	|
	symbol comma stringvals {
		$$.lbAlgorithm = $1.token
		$$.lbEndpoints = $3.stringvals
	}

lbbackend:
	openarrow lbbackendbody closearrow {
		$$.lbAlgorithm = $2.lbAlgorithm
		$$.lbEndpoints = $2.lbEndpoints
	}

backend:
	stringval {
		$$.backend = $1.stringval
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = false
	}
	|
	shunt {
		$$.shunt = true
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = false
	}
	|
	loopback {
		$$.shunt = false
		$$.loopback = true
		$$.dynamic = false
		$$.lbBackend = false
	}
	|
	dynamic {
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = true
		$$.lbBackend = false
	}
	|
	lbbackend {
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = true
		$$.lbAlgorithm = $1.lbAlgorithm
		$$.lbEndpoints = $1.lbEndpoints
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
