done = function(summary, latency, requests)
  local report="result.csv"
  local f=io.open(report,"r")
  if f~=nil then
     io.close(f)
  else
     f = io.open(report, "a+")
     f:write("min,max,mean,stdev,p50,p90,p99,p99999,duration,requests,bytes\n")
     io.close(f)
  end

  -- open output file
  f = io.open(report, "a+")

  -- write below results to file
  --   minimum latency
  --   max latency
  --   mean of latency
  --   standard deviation of latency
  --   50percentile latency
  --   90percentile latency
  --   99percentile latency
  --   99.999percentile latency
  --   duration of the benchmark
  --   total requests during the benchmark
  --   total received bytes during the benchmark

  f:write(string.format("%f,%f,%f,%f,%f,%f,%f,%f,%d,%d,%d\n",
  latency.min, latency.max, latency.mean, latency.stdev, latency:percentile(50),
  latency:percentile(90), latency:percentile(99), latency:percentile(99.999),
  summary["duration"], summary["requests"], summary["bytes"]))

  f:close()
end
