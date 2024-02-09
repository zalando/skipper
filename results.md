`go test -benchmem -run=^$ -bench ^BenchmarkStripQuery$ -benchtime=10s  github.com/zalando/skipper/filters/builtin`

```
BenchmarkStripQuery/[old_sanitize]_url_1-8              85732707               118.6 ns/op            80 B/op          4 allocs/op
BenchmarkStripQuery/[old_sanitize]_url_2-8              14885370               808.2 ns/op           472 B/op         15 allocs/op
BenchmarkStripQuery/[old_sanitize]_url_3-8              73297474               162.9 ns/op           120 B/op          4 allocs/op
BenchmarkStripQuery/[old_sanitize]_url_4-8              100000000              114.1 ns/op            80 B/op          4 allocs/op


BenchmarkStripQuery/[new_sanitize]_url_1-8              100000000              103.5 ns/op            24 B/op          3 allocs/op
BenchmarkStripQuery/[new_sanitize]_url_2-8              15854868               778.5 ns/op           192 B/op         14 allocs/op
BenchmarkStripQuery/[new_sanitize]_url_3-8              71689101               166.3 ns/op            64 B/op          4 allocs/op
BenchmarkStripQuery/[new_sanitize]_url_4-8              100000000              102.0 ns/op            24 B/op          3 allocs/op
```