---
name: go-coding
description: Create or modify Go skipper code
license: MIT
compatibility: opencode, claude
metadata:
  audience: maintainers, contributors
---
## What I do

- focus on Go code and architecture

## When to use me

Use this when you are reading or writing Go code

## Quick start

Use `gopls` to navigate the code.

### Package and type organization

Group related types together using a single `type (...)` block. Use iota for
enumerated constants. Keep unexported types lowercase; only export what callers
need.

```go
type (
    matchBehavior int
    matchMode     int
)

const (
    matchBehaviorAll matchBehavior = iota
    matchBehaviorAny

    matchModeExact matchMode = iota
    matchModeRegexp
)

type (
    spec struct {
        name          string
        matchBehavior matchBehavior
        matchMode     matchMode
        reg           *registry
    }
    predicate struct {
        kv            map[string][]valueMatcher
        matchBehavior matchBehavior
        cache         sync.Map
    }
)
```

### Precompute in Create/New, not in the hot path

`CreateFilter` and `Create` run at route-build time. Precompute everything
costly there (compile regexps, build maps, normalize config). The hot path
(`Request`, `Response`, `Match`) must be allocation-free on the common case.

```go
func (s *spec) Create(args []any) (routing.Predicate, error) {
    if len(args) == 0 || len(args)%2 != 0 {
        return nil, predicates.ErrInvalidPredicateParameters
    }
    kv := make(map[string][]valueMatcher)
    for i := 0; i < len(args); i += 2 {
        key, keyOk := args[i].(string)
        value, valueOk := args[i+1].(string)
        if !keyOk || !valueOk {
            return nil, predicates.ErrInvalidPredicateParameters
        }
        re, err := regexp.Compile(value)
        if err != nil {
            return nil, predicates.ErrInvalidPredicateParameters
        }
        kv[key] = append(kv[key], regexMatcher{regexp: re})
    }
    return &predicate{kv: kv}, nil
}
```

### Cache expensive Create results to survive frequent route table rebuilds

Route tables rebuild every few seconds. Cache `CreateFilter`/`Create` results
keyed by config args to avoid recreating identical instances and leaking memory.
Use `sync.Mutex` around the map; check-then-insert is fine for idempotent
objects.

```go
type spec struct {
    mu        sync.Mutex
    filterMap map[string]*myFilter
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
    key, err := keyFromArgs(args)
    if err != nil {
        return nil, err
    }
    s.mu.Lock()
    if f, ok := s.filterMap[key]; ok {
        s.mu.Unlock()
        return f, nil
    }
    s.mu.Unlock()
    // ... build f ...
    s.mu.Lock()
    s.filterMap[key] = f
    s.mu.Unlock()
    return f, nil
}
```

### Cache hot-path lookups with sync.Map

For repeated lookups in `Match`/`Request`/`Response` (e.g. parsed tokens,
validated values), use `sync.Map` — it avoids a single global mutex bottleneck
under heavy read concurrency. Periodically clear it rather than tracking
per-entry TTLs; infrequent full clears are cheap.

```go
type predicate struct {
    kv    map[string][]valueMatcher
    cache sync.Map // wire-string → bool
}

// in a background goroutine started at spec construction time:
func (r *registry) clean() {
    tick := time.NewTicker(time.Hour)
    defer tick.Stop()
    for {
        select {
        case <-r.quit:
            return
        case <-tick.C:
            r.mu.Lock()
            for _, p := range r.predicateMap {
                p.cache.Clear()
            }
            r.mu.Unlock()
        }
    }
}
```

### Avoid allocations in string parsing

Prefer `strings.Index` / `strings.Cut` over `strings.Split` to avoid
allocating a slice. Use iterator-style helpers (or `strings.SplitSeq` in
go1.24+) to range over parts without materializing them.

```go
// zero-allocation split iteration (backport of strings.SplitSeq)
func splitSeq(s, sep string) func(yield func(string) bool) {
    return func(yield func(string) bool) {
        for {
            i := strings.Index(s, sep)
            if i < 0 {
                break
            }
            if !yield(s[:i]) {
                return
            }
            s = s[i+len(sep):]
        }
        yield(s)
    }
}

// usage
for part := range splitSeq(header, ",") {
    token, value, found := strings.Cut(strings.TrimSpace(part), "=")
    _ = token; _ = value; _ = found
}
```

### Skip work when input hasn't changed

For dataclients and watchers, record the last successful input (e.g. file
bytes, ETag) and return early without parsing when nothing changed.

```go
func (c *WatchClient) loadUpdates() watchResponse {
    content, err := os.ReadFile(c.fileName)
    if err != nil { ... }

    if bytes.Equal(content, c.lastContent) {
        return watchResponse{} // no change
    }
    r, err := eskip.Parse(string(content))
    if err != nil {
        c.lastContent = nil
        return watchResponse{err: err}
    }
    c.lastContent = content
    ...
}
```

### Error handling

Wrap errors with context using `fmt.Errorf("...: %w", err)`. Return sentinel
errors (`predicates.ErrInvalidPredicateParameters`) from `Create`/`CreateFilter`
for invalid config so callers can distinguish config errors from runtime errors.
Never swallow errors in the hot path.

```go
req, endpointMetrics, err := p.mapRequest(ctx, requestContext)
if err != nil {
    return nil, &proxyError{err: fmt.Errorf("could not map backend request: %w", err)}
}
```
