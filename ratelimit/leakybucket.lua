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

-- The leaky bucket state is stored as a single value: the timestamp when the bucket will be empty.
-- An increment is allowed if it does not cause the bucket to overflow its capacity.
-- The bucket's capacity can be expressed as the maximum time-to-drain, which is `capacity * emission`.

-- Calculate `new_empty_at`: the time when the bucket would be empty if the increment is added.
local new_empty_at = empty_at + increment * emission

-- Calculate `max_empty_at`: the latest possible drain time if the bucket was filled to capacity now.
local max_empty_at = now + capacity * emission

-- If `new_empty_at` does not exceed `max_empty_at`, the increment is allowed.
if new_empty_at <= max_empty_at then
    redis.call("SET", bucket_id, new_empty_at, "PX", math.ceil((new_empty_at - now) / 1000))
    return 1 -- success
end

-- If not allowed, return a negative value indicating the number of microseconds to wait for a retry,
-- which is the time until enough units have leaked to accommodate the increment.
return max_empty_at - new_empty_at
