-- stripQuery("true")
function request(ctx, params)
	if params[1] == "true" then
		for k, v in ctx.request.url_query() do
			ctx.request.header["X-Query-Param-" .. k] = v
		end
	end
	ctx.request.url_raw_query = ""
end
