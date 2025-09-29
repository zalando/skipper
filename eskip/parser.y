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
	forward bool
	numval float64
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
%token forward
%token stringliteral
%token symbol
%token openarrow
%token closearrow

%token start_document;
%token start_predicates;
%token start_filters;

%%

start:
	start_document document {
		eskiplex.(*eskipLex).routes = $2.routes
	}
	|
	start_predicates {
		// allow empty or comments only
		eskiplex.(*eskipLex).predicates = nil
	}
	|
	start_predicates predicates {
		eskiplex.(*eskipLex).predicates = $2.predicates
	}
	|
	start_filters {
		// allow empty or comments only
		eskiplex.(*eskipLex).filters = nil
	}
	|
	start_filters filters {
		eskiplex.(*eskipLex).filters = $2.filters
	}

document:
	routes {
		$$.routes = $1.routes
	}
	|
	route {
		$$.routes = []*parsedRoute{$1.route}
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
			forward: $3.forward,
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
			forward: $5.forward,
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
	stringliteral {
		$$.arg = $1.token
	}
	|
	regexpliteral {
		$$.arg = $1.token
	}

stringvals:
	stringliteral {
		$$.stringvals = []string{$1.token}
	}
	|
	stringvals comma stringliteral {
		$$.stringvals = $1.stringvals
		$$.stringvals = append($$.stringvals, $3.token)
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
	stringliteral {
		$$.backend = $1.token
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = false
		$$.forward = false
	}
	|
	shunt {
		$$.shunt = true
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = false
		$$.forward = false
	}
	|
	loopback {
		$$.shunt = false
		$$.loopback = true
		$$.dynamic = false
		$$.lbBackend = false
		$$.forward = false
	}
	|
	dynamic {
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = true
		$$.lbBackend = false
		$$.forward = false
	}
	|
	lbbackend {
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = true
		$$.lbAlgorithm = $1.lbAlgorithm
		$$.lbEndpoints = $1.lbEndpoints
		$$.forward = false
	}
	|
	forward {
		$$.shunt = false
		$$.loopback = false
		$$.dynamic = false
		$$.lbBackend = false
		$$.forward = true
	}

numval:
	number {
		$$.numval = convertNumber($1.token)
	}

%%
