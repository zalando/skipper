package apiusagemonitoring

// apiConfig is the structure used to parse the parameters of the filter.
type apiConfig struct {
	ApplicationId         string   `json:"application_id"`
	ApiId                 string   `json:"api_id"`
	PathTemplates         []string `json:"path_templates"`
	ClientTrackingPattern string   `json:"client_tracking_pattern"`
}
