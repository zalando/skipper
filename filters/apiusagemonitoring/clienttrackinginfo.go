package apiusagemonitoring

import "regexp"

type clientTrackingInfo struct {
	ClientTrackingMatcher   *regexp.Regexp
	RealmKey                string
	ClientIdKey             string
}
