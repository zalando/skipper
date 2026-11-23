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
		l := eskiplex.(*eskipLex)
		l.predicates = l.takePredicates()
	}
	|
	start_filters {
		// allow empty or comments only
		eskiplex.(*eskipLex).filters = nil
	}
	|
	start_filters filters {
		l := eskiplex.(*eskipLex)
		l.filters = l.takeFilters()
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
		l := eskiplex.(*eskipLex)
		r := l.newParsedRoute()
		r.predicates = l.takePredicates()
		r.backend = $3.backend
		r.shunt = $3.shunt
		r.loopback = $3.loopback
		r.dynamic = $3.dynamic
		r.lbBackend = $3.lbBackend
		r.lbAlgorithm = $3.lbAlgorithm
		r.lbEndpoints = $3.lbEndpoints
		r.forward = $3.forward
		$$.route = r
		$3.lbEndpoints = nil
	}
	|
	predicates arrow filters arrow backend {
		l := eskiplex.(*eskipLex)
		r := l.newParsedRoute()
		r.predicates = l.takePredicates()
		r.filters = l.takeFilters()
		r.backend = $5.backend
		r.shunt = $5.shunt
		r.loopback = $5.loopback
		r.dynamic = $5.dynamic
		r.lbBackend = $5.lbBackend
		r.lbAlgorithm = $5.lbAlgorithm
		r.lbEndpoints = $5.lbEndpoints
		r.forward = $5.forward
		$$.route = r
		$5.lbEndpoints = nil
	}

predicates:
	predicate {
		l := eskiplex.(*eskipLex)
		l.predicatesBuf = append(l.predicatesBuf[:0], $1.predicate)
	}
	|
	predicates and predicate {
		l := eskiplex.(*eskipLex)
		l.predicatesBuf = append(l.predicatesBuf, $3.predicate)
	}

predicate:
	any {
		$$.predicate = eskiplex.(*eskipLex).newPredicate("*", nil)
	}
	|
	symbol openparen args closeparen {
		l := eskiplex.(*eskipLex)
		$$.predicate = l.newPredicate($1.token, l.takeArgs())
	}

filters:
	filter {
		l := eskiplex.(*eskipLex)
		l.filtersBuf = append(l.filtersBuf[:0], $1.filter)
	}
	|
	filters arrow filter {
		l := eskiplex.(*eskipLex)
		l.filtersBuf = append(l.filtersBuf, $3.filter)
	}

filter:
	symbol openparen args closeparen {
		l := eskiplex.(*eskipLex)
		$$.filter = l.newFilter($1.token, l.takeArgs())
	}

args:
	{
		eskiplex.(*eskipLex).argsBuf = eskiplex.(*eskipLex).argsBuf[:0]
	}
	|
	arg {
		l := eskiplex.(*eskipLex)
		l.argsBuf = append(l.argsBuf[:0], $1.arg)
	}
	|
	args comma arg {
		l := eskiplex.(*eskipLex)
		l.argsBuf = append(l.argsBuf, $3.arg)
	}

arg:
	numval {
		$$.arg = eskiplex.(*eskipLex).internFloatAny($1.numval)
	}
	|
	stringliteral {
		$$.arg = eskiplex.(*eskipLex).internStringAny($1.token)
	}
	|
	regexpliteral {
		$$.arg = eskiplex.(*eskipLex).internStringAny($1.token)
	}

stringvals:
	stringliteral {
		l := eskiplex.(*eskipLex)
		l.stringvalsBuf = append(l.stringvalsBuf[:0], $1.token)
	}
	|
	stringvals comma stringliteral {
		l := eskiplex.(*eskipLex)
		l.stringvalsBuf = append(l.stringvalsBuf, $3.token)
	}

lbbackendbody:
	stringvals {
		$$.lbEndpoints = eskiplex.(*eskipLex).takeStringvals()
	}
	|
	symbol comma stringvals {
		$$.lbAlgorithm = $1.token
		$$.lbEndpoints = eskiplex.(*eskipLex).takeStringvals()
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
