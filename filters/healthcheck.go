package filters

type HealthCheck struct{}

func (h *HealthCheck) Name() string                                 { return "healthcheck" }
func (h *HealthCheck) CreateFilter(_ []interface{}) (Filter, error) { return h, nil }
func (h *HealthCheck) Request(ctx FilterContext)                    {}
func (h *HealthCheck) Response(ctx FilterContext)                   { ctx.Response().StatusCode = 200 }
