-- randomPath("/prefix/", "10", "/optional/suffix")
function request(ctx, params)
	local p = params[1]
	local n = tonumber(params[2])
	for i = 1, n do
		p = p .. random_char()
	end
	if params[3] ~= nil then
		p = p .. params[3]
	end
	ctx.request.url_path = p
end

local charset = "abcdefghijklmnopqrstuvwxyz"

function random_char()
	local r = math.random(1, #charset)
	return charset:sub(r, r)
end
