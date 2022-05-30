local bucket_id = KEYS[1]           -- bucket id
local capacity = tonumber(ARGV[1])  -- bucket capacity in units (increment <= capacity)
local emission = tonumber(ARGV[2])  -- time to leak one unit in microseconds (emission > 0)
local increment = tonumber(ARGV[3]) -- increment in units (increment <= capacity)
local now = tonumber(ARGV[4])       -- current time in microseconds (now >= 0)

-- Redis stores the timestamp when bucket drains out.
-- Lua uses double floating-point as a number type which can precisely represent integers only up to 2^53.
-- The timestamp is stored in microseconds (and not nanoseconds) to keep values below 2^53.
-- If bucket does not exist or is drained out, consider it empty now.
local empty_at = redis.call("GET", bucket_id)
if not empty_at then
    empty_at = now
else
    empty_at = tonumber(empty_at)
    if empty_at < now then
        empty_at = now
    end
end

-- bucket level == time to drain / emission == (empty_at - now) / emission
-- free capacity left after increment == capacity - bucket level - increment
-- If free capacity is negative then retry is possible after -(free capacity * emission)
-- Calculate and check the value of x == free capacity * emission
local x = (capacity - increment) * emission - (empty_at - now)
if x >= 0 then
    empty_at = empty_at + increment * emission

    redis.call("SET", bucket_id, empty_at, "PX", math.ceil((empty_at - now) / 1000))
end

return x
