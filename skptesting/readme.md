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
skptestint/print-cpu-profile.sh
```

Prints the CPU profile saved from the last run of skptesting/profile-proxy.sh.

## Print Memory Profile

```
skptestint/print-mem-profile.sh
```

Prints the CPU profile saved from the last run of skptesting/profile-proxy.sh.

## Self-signed Certificate

```
skptesting/self-cert.sh
```

Generates a self-signed TLS key and certificate.
