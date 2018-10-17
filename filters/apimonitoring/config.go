package apimonitoring

// filterConfig is the structure of the filter parameter (the JSON object)
// describing the configuration of one API Monitoring filter.
type filterConfig struct {
	Apis []*apiConfig `json:"apis"`
}

type apiConfig struct {
	ApplicationId string   `json:"application_id"`
	ApiId         string   `json:"api_id"`
	PathTemplates []string `json:"path_templates"`
}
