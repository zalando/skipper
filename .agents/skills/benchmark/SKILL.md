---
name: benchmark
description: Create or modify hot-path code or optimizations should be validated by a Go Benchmark
license: MIT
compatibility: opencode, claude
metadata:
  audience: maintainers, contributors
---
## What I do

- focus on performance and allocations

## When to use me

Use this when you are creating or modifying code in the hot path.

## Quick start

### Benchmarks

Write benchmarks alongside any performance-sensitive change. Use `benchstat`
with `-count 20` to compare. Always report `sec/op`, `B/op`, and `allocs/op`.
Target zero allocations in hot-path code.

```go
func BenchmarkMyPredicate(b *testing.B) {
    p := &predicate{...}
    req := httptest.NewRequest("GET", "/", nil)
    req.Header.Set("Authorization", "Bearer "+testToken)
    b.ReportAllocs()
    b.ResetTimer()
    for b.Loop() {
        p.Match(req)
    }
}
```

Example run:

```bash
% go test -bench=BenchmarkMyPredicate -benchmem -run='^$' -count 20 -cpu 1,2,4,8,16 ./predicates/mypredicate > old.txt
# ..edit code..
% go test -bench=BenchmarkMyPredicate -benchmem -run='^$' -count 20 -cpu 1,2,4,8,16 ./predicates/mypredicate > new.txt
% benchstat old.txt new.txt
```

Show the output of benchstat to the developer.
