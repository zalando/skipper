# Flow ID Filter

Flow IDs let you correlate router logs for a given request against the upstream application logs for that same request.
If your upstream application makes other requests to other services it can provide the same Flow ID value so that all
of those logs can be correlated.

## How it works
Skipper generates a unique Flow ID for every HTTP request that it receives. The Flow ID is then passed to your 
upstream application as an HTTP header called X-Flow-Id.

## Some benchmarks

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
	
The current implementation just gets len / 2 (hex.DecodedLen) bytes from the PRNG and hex encodes them. 
Its performance is only dependent on the length of the generated FlowId, according to the following benchmarks:

	BenchmarkFlowIdLen8-4 	 1000000	      1157 ns/op
	BenchmarkFlowIdLen10-4	 1000000	      1162 ns/op
	BenchmarkFlowIdLen12-4	 1000000	      1163 ns/op
	BenchmarkFlowIdLen14-4	 1000000	      1171 ns/op
	BenchmarkFlowIdLen16-4	 1000000	      1180 ns/op
	BenchmarkFlowIdLen32-4	 1000000	      1957 ns/op
	BenchmarkFlowIdLen64-4	  300000	      3520 ns/op

As you can see, starting at len = 32 (16 random bytes) the performance starts dropping dramatically. 