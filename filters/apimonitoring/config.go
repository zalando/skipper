package apimonitoring

type filterConfig struct {
	Verbose       bool     `json:"verbose"`
	ApplicationId string   `json:"application_id"`
	PathTemplates []string `json:"path_templates"`
}
