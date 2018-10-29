package apiusagemonitoring

import "regexp"

type clientTrackingInfo struct {
	ClientTrackingMatcher   *regexp.Regexp
	RealmAndClientIdMatcher *regexp.Regexp
	RealmKey                string
	ClientIdKey             string
}
