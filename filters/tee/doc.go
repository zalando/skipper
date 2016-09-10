/*
Package tee provides a unix-like tee feature for routing. One can send same request to a shadow backend
by using Tee filter.

An example can be as follow
 * -> Tee("http://audit-logging.example.com") -> "http://payment.example.com"

 This will snd same request for payment.example.com to audit-logging.example.com.
 Another use case could be using it for benchmarking a new backend with some real traffic

 It is also possible using it with a regexp like in modpath filter

 Path("/api/v1") -> Tee("https://api.example.com", ".*", "/v2/" ) -> "http://example.org/"

 In this example. once can tests a new version of the backend with Tee.

*/
package tee
