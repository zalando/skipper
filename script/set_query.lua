-- setQuery("k", "v")
function request(ctx, params)
	ctx.request.url_query[params[1]] = params[2]
end
