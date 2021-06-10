-- queryToHeader("foo-query-param", "X-Foo-Header")
function request(ctx, params)
	-- do not overwrite
	if ctx.request.header[params[2]] == "" then
		local v = ctx.request.url_query[params[1]]
		-- set if present
		if v ~= nil then
			ctx.request.header[params[2]] = v
		end
	end
end
