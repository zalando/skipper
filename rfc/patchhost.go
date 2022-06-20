package rfc

import "strings"

// PatchHost returns a host string without trailing dot. For details
// see also the discussion in
// https://lists.w3.org/Archives/Public/ietf-http-wg/2016JanMar/0430.html.
func PatchHost(host string) string {
	host = strings.ReplaceAll(host, ".:", ":")
	return strings.TrimSuffix(host, ".")
}
