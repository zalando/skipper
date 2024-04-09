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

function headers_as_table(header)
	local t = {}
	for k, _ in header() do
		if k:lower() == "authorization" then
			table.insert(t, k)
			table.insert(t, ": TRUNCATED\r\n")
		else
			table.insert(t, k)
			table.insert(t, ": ")
			table.insert(t, table.concat(header.values(k), " "))
			table.insert(t, "\r\n")
		end
	end
	table.insert(t, "\r\n")
	return t
end
