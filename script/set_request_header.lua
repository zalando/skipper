-- setRequestHeader("header-name", "header-value")
function request(ctx, params)
	ctx.request.header[params[1]] = params[2]
	if params[1]:lower() == "host" then
		ctx.request.outgoing_host = params[2]
	end
end
