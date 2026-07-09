# Extended Testing/Benchmarking

This directory may contain additional testing tools and data that fall outside of the standard Go testing
methods. The tools in this directory may have dependencies that come with limitations on the platform and
environment that they are running in.

Some of the scripts contain commented instructions to be able to compare the results with other similar
utilities, e.g. nginx. Feel free to mod these scripts.

WARNING: in the following benchmarks, some of the error messages are muted, which means that, when done,
the transfer rate needs to be verified in the output.

Dependencies: bash, wrk. For generating TLS keys, openssl.

## Benchmark Proxy

```
skptesting/benchmark-proxy.sh 12 128 3
```

Benchmarks skipper as a proxy with a static file server behind it, running for 12s, over 128 connections and
with a preliminary warmup time of 3s.

## Benchmark Proxy - TLS

```
skptesting/benchmark-proxy-tls.sh 12 128 3
```

Benchmarks skipper as a proxy with a TLS enabled static file server behind it, running for 12s, over 128
connections and with a preliminary warmup time of 3s.

## Benchmark Static

```
skptesting/benchmark-static.sh 12 128 3
```

Benchmarks skipper as a static file server, running for 12s, over 128 connections and with a preliminary warmup
time of 3s.

## Benchmark Compress

```
skptesting/benchmark-compress.sh 12 128 3
```

Benchmarks skipper as an HTTP compression proxy with a static file server behind it, running for 12s, over 128
connections and with a preliminary warmup time of 3s.

## Benchmark Load Balancer Algorithms

```
skptesting/benchmark-lb-algorithms.sh 1000 2m 128 "roundRobin weightedRoundRobin"
```

Benchmarks the load balancer algorithms end to end against a large number of local backend processes to
measure the algorithm overhead and lock contention under sustained load, here 1000 backends, for 2 minutes,
over 128 connections. The backends are instances of a minimal C HTTP server (okserver.c) that is compiled
automatically. See the script header for the environment variables that control the ports and the number of
backend ports served per one process, for systems with a low process limit. Requires a C compiler, and wrk
or hey (https://github.com/rakyll/hey) as the load generator.

## Set CPU Frequency Scaling

Set the system CPU scaling governor to best performance:

```
skptesting/cpu 8 performance
```

(It may need root permissions.)

The above command writes 'performance' into the cpufreq/scaling_governor value of first 8 cpu entries in
/sys/devices/system/cpu. In case the system is a workstation, one may want to set it back to the typical default
mode:

```
skptesting/cpu 8 powersave
```

## Profile Proxy

```
skptesting/profile-proxy.sh 12 128
```

Generates CPU and memory profile for Skipper as a proxy, with a static file server behind it, running for 12s,
over 128 connections. Use skptesting/print-cpu-profile.sh and skptesting/print-mem-profile.sh to get the
results.

## Print CPU Profile

```
skptesting/print-cpu-profile.sh
```

Prints the CPU profile saved from the last run of skptesting/profile-proxy.sh.

## Print Memory Profile

```
skptesting/print-mem-profile.sh
```

Prints the CPU profile saved from the last run of skptesting/profile-proxy.sh.

## Self-signed Certificate

```
skptesting/self-cert.sh
```

Generates a self-signed TLS key and certificate.
