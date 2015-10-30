/*
Package flowid implements a filter used for identifying incoming requests through their complete lifecycle for
logging and monitoring or else.

Flow Ids let you correlate router logs for a given request against the upstream application logs for that same request.
If your upstream application makes other requests to other services it can provide the same Flow Id value so that all
of those logs can be correlated.


How It Works

Skipper generates a unique Flow Id for every HTTP request that it receives. The Flow ID is then passed to your
upstream application as an HTTP header called X-Flow-Id.

The filter takes 2 optional parameters:
	1. Accept existing X-Flow-Id header
	2. Flow Id length

The first parameter is a string parameter that, when set to "reuse", will make the filter skip the generation of
a new flow id. If the existing header value is not a valid flow id it is ignored and a new flow id is also generated.
Any other string used for this parameter is ignored and have the same meaning - not to accept existing X-Flow-Id
headers.

The second parameter is a number that defines the length of the generated flow ids. Valid options are any even number
between 8 and 64.


Usage

The filter can be used with many different combinations of parameters. It can also be used without any parameter, since
both are options.

Default parameters

	flowId()

Without any parameters, the filter doesn't reuse existing X-Flow-Id headers and generates new ones with 16 bytes.

Reuse existing flow id

	flowId("reuse")

With only the first parameter with the string "reuse" the filter will accept an existing X-Flow-Id header, if
it's present in the request.

Generate bigger flow ids

	flowId("fo shizzle", 64)

This example doesn't accept a X-Flow-Id header and will always generate new flow ids with 64 bytes.


Some Benchmarks

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

	BenchmarkFlowIdLen8-4 	10000000	       159 ns/op
	BenchmarkFlowIdLen10-4	10000000	       164 ns/op
	BenchmarkFlowIdLen12-4	10000000	       202 ns/op
	BenchmarkFlowIdLen14-4	10000000	       206 ns/op
	BenchmarkFlowIdLen16-4	10000000	       216 ns/op
	BenchmarkFlowIdLen32-4	 5000000	       329 ns/op
	BenchmarkFlowIdLen64-4	 3000000	       532 ns/op

*/
package flowid
