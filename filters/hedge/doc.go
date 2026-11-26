/*
Package hedge provides the disableHedge filter and the preprocessor that
excludes tee and shadow traffic routes from hedged requests.

Hedged requests are enabled globally by the -hedge-enabled flag. A route
can opt out with the disableHedge filter:

	r: * -> disableHedge() -> <roundRobin, "http://a.example.org", "http://b.example.org">;

Routes that use tee, teenf, teeLoopback or teeResponse filters, or the Tee
predicate, are never hedged. The preprocessor adds disableHedge to such
routes.
*/
package hedge
