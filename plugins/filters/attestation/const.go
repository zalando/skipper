package main

import (
	"regexp"
)

const (
	minimumAndroidVersion = "v7.41.0"
	minimumIosVersion     = "v7.51.0"
	production            = "production"
	dev                   = "dev"
	local                 = "local"

	productionAndroidPackageName       = "com.muzmatch.muzmatchapp"
	productionAndroidSigningCertDigest = "dpkBP6sRbN7Cu7B7Rv0AvxQPSZzOYJ9u-Gn5zYs_pWI"
	debugAndroidPackageName            = "com.muzmatch.muzmatchapp.debug"
	debugAndroidSigningCertDigest      = "wV4SYt84cgGObwVuCfLBGYmTplP_wNDk6H5_ng6sZcc"
)

type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIos     Platform = "ios"
)

type integrityEvaluation int

const (
	integrityUnevaluated integrityEvaluation = iota
	integrityFailure
	integritySuccess
)

var (
	iOSUserAgents = []*regexp.Regexp{
		regexp.MustCompile(`^Muzz/[7-8]\.\d+\.\d+ \(com\.muzmatch\.muzmatch; build:\d+; iOS \d+\.\d+\.\d+\) Alamofire/\d+\.\d+\.\d+$`),
		regexp.MustCompile(`^MuzzAlpha/[7-8]\.\d+\.\d+ \(com\.muzmatch\.muzmatch\.alpha; build:\d+; iOS \d+\.\d+\.\d+\) Alamofire/\d+\.\d+\.\d+$`),
		regexp.MustCompile(`^MuzzTestsUI-Runner/\d+\.\d+ \(com\.muzmatch\.muzmatchUITests\.xctrunner; build:\d+; iOS \d+\.\d+\.\d+\) Alamofire/\d+\.\d+\.\d+$`),
	}
	androidUserAgent = regexp.MustCompile(`^okhttp/\d+\.\d+\.\d+$`)
)
