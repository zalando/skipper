/*
Package eskipfile implements the DataClient interface for reading the skipper route definitions from an eskip
formatted file.

(See the DataClient interface in the skipper/routing package and the eskip
format in the skipper/eskip package.)

The package provides two implementations: one without file watch (legacy version) and one with file watch. When
running the skipper command, the one with watch is used.
*/
package eskipfile
