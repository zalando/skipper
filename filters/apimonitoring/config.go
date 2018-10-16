package apimonitoring

type filterConfig struct {
	ApplicationId string   `json:"application_id"`
	PathTemplates []string `json:"path_templates"`
}
