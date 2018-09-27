package apimonitoring

type filterConfig struct {
	Apis    []*filterConfigApi `json:"apis"`
	Verbose bool               `json:"verbose"`
}

type filterConfigApi struct {
	Id            string   `json:"id"`
	ApplicationId string   `json:"application_id"`
	PathPatterns  []string `json:"path_patterns"`
}
