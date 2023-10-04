package kubernetes

import (
	"net/url"
	"regexp"
	"strings"
)

func createHostRx(hosts ...string) string {
	if len(hosts) == 0 {
		return ""
	}

	hrx := make([]string, len(hosts))
	for i, host := range hosts {
		// trailing dots and port are not allowed in kube
		// ingress spec, so we can append optional setting
		// without check
		hrx[i] = strings.ReplaceAll(host, ".", "[.]") + "[.]?(:[0-9]+)?"
	}

	return "^(" + strings.Join(hrx, "|") + ")$"
}

func isExternalDomainAllowed(allowedDomains []*regexp.Regexp, domain string) bool {
	for _, a := range allowedDomains {
		if a.MatchString(domain) {
			return true
		}
	}

	return false
}

func isExternalAddressAllowed(allowedDomains []*regexp.Regexp, address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}

	return isExternalDomainAllowed(allowedDomains, u.Hostname())
}
