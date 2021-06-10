-- copyRequestHeader("X-From", "X-To")
function request(ctx, params)
	local value = ctx.request.header[params[1]]
	if value ~= "" then
		ctx.request.header[params[2]] = value
		if params[2]:lower() == "host" then
			ctx.request.outgoing_host = value
		end
	end
end
