/*
Package flowid implements a filter used for identifying incoming requests through their complete lifecycle for
logging and monitoring or else.

Flow Ids let you correlate router logs for a given request against the upstream application logs for that same request.
If your upstream application makes other requests to other services it can provide the same Flow ID value so that all
of those logs can be correlated.

# How It Works

Skipper generates a unique Flow ID for every HTTP request that it receives. The Flow ID is then passed to your
upstream application as an HTTP header called X-Flow-Id.

The filter takes 1 optional string parameter that, when set to "reuse", will make the filter check for the presence
of another FlowID already set in the inbound request in a header with the same name. If the existing header value is
not a valid flow id it is ignored and a new flow id is generated, overwriting the previous one.
Any other string used for this parameter is ignored and trigger the same, default, behavior - to ignore any existing
X-Flow-Id header.

# Generators

The Flow ID generation can follow any format. Skipper provides two Generator implementations - Standard and ULID. They
offer different performance and options and you can choose which one you prefer.

# Standard Flow IDs

The Standard Generator uses a base 64 alphabet and can be configured to generate flow IDs with length between 8 and
64 chars. It is very fast for small length FlowIDs and uses a system shared entropy source. It is safe for concurrent
use.

# ULID Flow IDs

The ULID Generator relies on the great work from https://github.com/alizain/ulid and https://github.com/oklog/ulid. It
generates 26 char long Universally Unique Lexicographically Sortable IDs. It is very fast and it's also safe for
concurrent use.

# Programmatic Usage

To create a specification of the FlowID filter you can either create the default specification, which uses the Standard
Generator with a length of 16 chars as default or provide your own Generator instance.

Default spec with Standard Generator (16 char FlowIDs)

	New()

Custom spec with ULID Generator (26 char FlowIDs)

	g := NewULIDGenerator()
	NewWithGenerator(g)

Custom spec with your own Generator implementation

	myCustomGenerator := newCustomGenerator(arg1, arg2)
	NewWithGenerator(myCustomGenerator)

# Routing Usage

The filter can be used with many different combinations of parameters. It can also be used without any parameter, using
defaults

Default parameters

	flowId()

Without any parameters, the filter doesn't reuse existing X-Flow-Id headers and generates new ones for every request.

Reuse existing flow id

	flowId("reuse")

With a single string parameter with the value "reuse", the filter will accept an existing X-Flow-Id header, if
it's present in the request. If it's invalid, a new one is generated and the header is overwritten.

# Some Benchmarks

# Built-In Flow ID Generator

To decide upon which hashing mechanism to use we tested some versions of UUID v1 - v4 and some other implementations.
The results are as follow:

	Benchmark_uuidv1-4	            5000000	       281 ns/op
	Benchmark_uuidv2-4	            5000000	       284 ns/op
	Benchmark_uuidv3-4	            2000000	       605 ns/op
	Benchmark_uuidv4-4	            1000000	      1903 ns/op
	BenchmarkRndAndSprintf-4  	    500000	      3312 ns/op
	BenchmarkSha1-4 	            1000000	      2188 ns/op
	BenchmarkMd5-4  	            1000000	      2076 ns/op
	BenchmarkFnv-4  	            500000	      2223 ns/op

The next approach was just to get len / 2 (hex.DecodedLen) bytes from the crypto/rand and hex encode them.
The performance was only dependent on the length of the generated FlowId and it performed like to the following
benchmarks:

	BenchmarkFlowIdLen8-4 	 1000000	      1157 ns/op
	BenchmarkFlowIdLen10-4	 1000000	      1162 ns/op
	BenchmarkFlowIdLen12-4	 1000000	      1163 ns/op
	BenchmarkFlowIdLen14-4	 1000000	      1171 ns/op
	BenchmarkFlowIdLen16-4	 1000000	      1180 ns/op
	BenchmarkFlowIdLen32-4	 1000000	      1957 ns/op
	BenchmarkFlowIdLen64-4	  300000	      3520 ns/op

Starting at len = 32 (16 random bytes) the performance started to drop dramatically.

The current implementation defines a static alphabet and build the flowid using random indexes to get elements from
that alphabet. The initial approach was to get a random index for each element. The performance was:

	BenchmarkFlowIdLen8-4 	 5000000	       375 ns/op
	BenchmarkFlowIdLen10-4	 3000000	       446 ns/op
	BenchmarkFlowIdLen12-4	 3000000	       508 ns/op
	BenchmarkFlowIdLen14-4	 3000000	       579 ns/op
	BenchmarkFlowIdLen16-4	 2000000	       641 ns/op
	BenchmarkFlowIdLen32-4	 1000000	      1179 ns/op
	BenchmarkFlowIdLen64-4	 1000000	      2268 ns/op

It was possible to optimize this behavior by getting a 64 bit random value and use every 6 bits (a total
of 10 usable random indexes) to get an element from the alphabet. This strategy improved the performance to the
following results:

	BenchmarkFlowIdStandardGenerator/8-8           	20000000	       104 ns/op	      16 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/10-8          	10000000	       121 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/12-8          	10000000	       155 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/14-8          	10000000	       158 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/16-8          	10000000	       168 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/26-8          	10000000	       224 ns/op	      64 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/32-8          	 5000000	       262 ns/op	      64 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGenerator/64-8          	 3000000	       444 ns/op	     128 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/8-8 	 5000000	       239 ns/op	      16 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/10-8         	 5000000	       244 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/12-8         	 3000000	       469 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/14-8         	 3000000	       473 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/16-8         	 3000000	       473 ns/op	      32 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/26-8         	 2000000	       701 ns/op	      64 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/32-8         	 2000000	       918 ns/op	      64 B/op	       2 allocs/op
	BenchmarkFlowIdStandardGeneratorInParallel/64-8         	 1000000	      1576 ns/op	     128 B/op	       2 allocs/op

ULID Flow ID Generator

	BenchmarkFlowIdULIDGenerator/Std-8                      	10000000	       194 ns/op	      48 B/op	       2 allocs/op
	BenchmarkFlowIdULIDGeneratorInParallel-8                	 5000000	       380 ns/op	      48 B/op	       2 allocs/op
*/
package flowid
