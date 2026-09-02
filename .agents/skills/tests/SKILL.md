---
name: test
description: Create tests that fit skipper code
license: MIT
compatibility: opencode, claude
metadata:
  audience: maintainers, contributors
---

## What I do

- Create test cases that matter

## When to use me

Use this when you are writing tests

## Quick start

Write table driven tests
```go
func TestSomething(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		want    string
		wantErr bool // or error
	}{
		{
			name:  "Test A",
			input: "my-input 1",
			want:  "my-output 1",
		},
		{
			name:    "Test B",
			input:   "my-input 2",
			wantErr: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
		  // test code here with tt.input and check output matches tt.want ot tt.wantErr to check for the error
		})
	}
}
```

Create meaningful tests. Skipper is a proxy, so a client connects to the proxy and the proxy sends a maybe modified request to the backend

```go
func TestSetRequestHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	    // inspect request. Test: request was correctly modified.
		if v := r.Header.Get("Foo"); v == "bar" {
			t.Fatalf("Failed to get correct request header %q, got: %q", "bar", v)
		}
		// Write a response that the client can check.
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	spec := builtin.NewSetRequestHeader()
	fr := make(filters.Registry)
	fr.Register(spec)
	r := eskip.MustParse(fmt.Sprintf(`* -> setRequestHeader("Foo", "bar") -> "%s"`, backend.URL))
	proxy := proxytest.WithParams(fr, proxy.Params{}, r...)
	defer proxy.Close()

	req, err := http.NewRequest("GET", proxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := proxy.Client().Transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("Failed to get response: %v", err)
	}

	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get correct status code 200, got: %d", rsp.StatusCode)
	}
}
```

If you need metrics.Metrics implementation to inspect metrics, you can use:

```go
mockMetrics := &metricstest.MockMetrics{}
// some code ..

// inspect counters
mockMetrics.WithCounters(func(counters map[string]int64) {
	if n := counters["a-counter"]; n == int64(5) { t.Fatalf("Failed to get expected counter value 5, got: %d", n) }
})

// inspect Gauges
mockMetrics.WithGauges(func(g map[string]float64) {
...
})
```
