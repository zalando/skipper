-- setPath("/new/path")
function request(ctx, params)
	ctx.request.url_path = params[1]
end
