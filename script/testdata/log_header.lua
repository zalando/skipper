-- logHeader("request", "response")
function request(ctx, params)
	if params["request"] ~= nil then
		local req = ctx.request
		print(req.method, " ", req.url, " ", req.proto, "\r\n",
			"Host: ", req.host, "\r\n",
			unpack(headers_as_table(req.header)))
	end
end

function response(ctx, params)
	if params["response"] ~= nil then
		local req = ctx.request
		local res = ctx.response
		print("Response for ", req.method, " ", req.url, " ", req.proto, "\r\n",
			res.status_code, "\r\n",
			unpack(headers_as_table(res.header)))
	end
end

function headers_as_table(iter)
	local t = {}
	for k, v in iter() do
		if k:lower() == "authorization" then
			table.insert(t, k)
			table.insert(t, ": TRUNCATED\r\n")
		else
			table.insert(t, k)
			table.insert(t, ": ")
			table.insert(t, v)
			table.insert(t, "\r\n")
		end
	end
	table.insert(t, "\r\n")
	return t
end
