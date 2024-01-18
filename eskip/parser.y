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
	predicates []*Predicate
	predicate *Predicate
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
	routeid route {
		$$.route = $2.route
		$$.route.id = $1.token
	}

routeid:
	symbol colon {
		// match symbol and colon to get route id early even if route parsing fails later
		$$.token = $1.token
		eskiplex.(*eskipLex).lastRouteID = $1.token
	}

route:
	predicates arrow backend {
		$$.route = &parsedRoute{
			predicates: $1.predicates,
			backend: $3.backend,
			shunt: $3.shunt,
			loopback: $3.loopback,
			dynamic: $3.dynamic,
			lbBackend: $3.lbBackend,
			lbAlgorithm: $3.lbAlgorithm,
			lbEndpoints: $3.lbEndpoints,
		}
		$1.predicates = nil
		$3.lbEndpoints = nil
	}
	|
	predicates arrow filters arrow backend {
		$$.route = &parsedRoute{
			predicates: $1.predicates,
			filters: $3.filters,
			backend: $5.backend,
			shunt: $5.shunt,
			loopback: $5.loopback,
			dynamic: $5.dynamic,
			lbBackend: $5.lbBackend,
			lbAlgorithm: $5.lbAlgorithm,
			lbEndpoints: $5.lbEndpoints,
		}
		$1.predicates = nil
		$3.filters = nil
		$5.lbEndpoints = nil
	}

predicates:
	predicate {
		$$.predicates = []*Predicate{$1.predicate}
	}
	|
	predicates and predicate {
		$$.predicates = $1.predicates
		$$.predicates = append($$.predicates, $3.predicate)
	}

predicate:
	any {
		$$.predicate = &Predicate{"*", nil}
	}
	|
	symbol openparen args closeparen {
		$$.predicate = &Predicate{$1.token, $3.args}
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
