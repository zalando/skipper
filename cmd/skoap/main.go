package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	logfilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/proxy"
)

const (
	addressFlag        = "address"
	defaultEtcdPrefix  = "/skipper"
	etcdPrefixFlag     = "etcd-prefix"
	etcdUrlsFlag       = "etcd-urls"
	targetAddressFlag  = "target-address"
	preserveHeaderFlag = "preserve-header"

	// TODO(sszuecs): move to skipper
	realmFlag     = "realm"
	scopesFlag    = "scopes"
	groupsFlag    = "groups"
	auditFlag     = "audit-log"
	auditBodyFlag = "audit-log-limit"

	routesFileFlag = "routes-file"
	insecureFlag   = "insecure"

	defaultAddress = ":9090"

	tlsCertFlag = "tls-cert"
	tlsKeyFlag  = "tls-key"

	verboseFlag = "v"

	experimentalUpgradeFlag = "experimental-upgrade"
)

const (
	usageHeader = `
skoap - Skipper based reverse proxy with authentication.

Use the skoap proxy to verify authorization tokens before forwarding requests, and optionally check OAuth2 realms
and scope or group membership. In addition to check incoming requests, optionally set basic authorization headers
for outgoing requests.

The command supports two modes:
- single route mode: when a target address is specified, only a single route is used and the authorization
  parameters (realm and scopes or groups) are specified as command line flags.
- routes configuration: supports any number of routes with custom predicate and filter settings. The
  authorization parameters are set in the routes file with the auth and authGroup filters.

When used with eskip configuration files, it is possible to apply detailed augmentation of the requests and
responses using Skipper rules.

https://github.com/zalando/skipper

`

	addressUsage    = `network address that skoap should listen on`
	etcdUrlsUsage   = "urls of nodes in an etcd cluster, storing route definitions"
	etcdPrefixUsage = "path prefix for skipper related data in etcd"

	targetAddressUsage = `when authenticating to a single network endpoint, set its address (without path) as
the -target-address`

	preserveHeaderUsage = `when forwarding requests, preserve the Authorization header in the outgoing request`

	realmUsage = `when target address is used to specify the target endpoint, and the requests need to be
authenticated against an OAuth2 realm, set the value of the realm with this flag. Note, that in case of a routes
file is used, the realm can be set for each auth filter reference individually`

	scopesUsage = `a comma separated list of the OAuth2 scopes to be checked in addition to the token validation
and the realm check`

	groupsUsage = `a comma separated list of the groups to be checked in addition to the token validation and the
realm check`

	auditUsage = `enable audit log in single route mode`

	auditBodyUsage = `set the limit of the audit log body`

	routesFileUsage = `alternatively to the target address, it is possible to use a full eskip route
configuration, and specify the auth() and authGroup() filters for the routes individually. See also:
https://godoc.org/github.com/zalando/skipper/eskip`

	insecureUsage = `when this flag set, skipper will skip TLS verification`

	// TODO
	certPathTLSUsage = "path of the certificate file"
	keyPathTLSUsage  = "path of the key"

	verboseUsage = `log level: Debug`

	experimentalUpgradeUsage = "enable experimental feature to handle upgrade protocol requests"
)

type singleRouteClient eskip.Route

var fs *flag.FlagSet

var (
	address             string
	etcdUrls            string
	etcdPrefix          string
	targetAddress       string
	preserveHeader      bool
	realm               string
	scopes              string
	groups              string
	audit               bool
	auditBody           int
	routesFile          string
	insecure            bool
	certPathTLS         string
	keyPathTLS          string
	verbose             bool
	experimentalUpgrade bool
)

func (src *singleRouteClient) LoadAll() ([]*eskip.Route, error) {
	return []*eskip.Route{(*eskip.Route)(src)}, nil
}

func (src *singleRouteClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	return nil, nil, nil
}

func usage() {
	fmt.Fprint(os.Stderr, usageHeader)
	fs.PrintDefaults()
}

func init() {
	fs = flag.NewFlagSet("flags", flag.ContinueOnError)
	fs.Usage = usage

	fs.StringVar(&address, addressFlag, defaultAddress, addressUsage)
	fs.StringVar(&etcdUrls, etcdUrlsFlag, "", etcdUrlsUsage)
	fs.StringVar(&etcdPrefix, etcdPrefixFlag, defaultEtcdPrefix, etcdPrefixUsage)
	fs.StringVar(&targetAddress, targetAddressFlag, "", targetAddressUsage)
	fs.BoolVar(&preserveHeader, preserveHeaderFlag, false, preserveHeaderUsage)
	fs.StringVar(&realm, realmFlag, "", realmUsage)
	fs.StringVar(&scopes, scopesFlag, "", scopesUsage)
	fs.StringVar(&groups, groupsFlag, "", groupsUsage)
	fs.BoolVar(&audit, auditFlag, false, auditUsage)
	fs.IntVar(&auditBody, auditBodyFlag, 1024, auditBodyUsage)
	fs.StringVar(&routesFile, routesFileFlag, "", routesFileUsage)
	fs.BoolVar(&insecure, insecureFlag, false, insecureUsage)
	fs.StringVar(&certPathTLS, tlsCertFlag, "", certPathTLSUsage)
	fs.StringVar(&keyPathTLS, tlsKeyFlag, "", keyPathTLSUsage)
	fs.BoolVar(&verbose, verboseFlag, false, verboseUsage)
	fs.BoolVar(&experimentalUpgrade, experimentalUpgradeFlag, false, experimentalUpgradeUsage)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}

		os.Exit(-1)
	}
}

func logUsage(message string) {
	fmt.Fprintf(os.Stderr, "%s\n", message)
	os.Exit(-1)
}

func main() {
	if !audit && auditBody != 1024 {
		logUsage("the audit-log-limit flag can be set only together with the audit-log flag")
	}

	o := skipper.Options{
		Address:    address,
		EtcdPrefix: etcdPrefix,
		CustomFilters: []filters.Spec{
			auth.NewAuth(auth.Options{AuthType: auth.AuthAllName}),
			auth.NewBasicAuth(),      // TODO(sszuecs): move to skipper
			logfilter.NewAuditLog()}, // TODO(sszuecs): move to skipper
		AccessLogDisabled:   true,
		ProxyOptions:        proxy.OptionsPreserveOriginal,
		CertPathTLS:         certPathTLS,
		KeyPathTLS:          keyPathTLS,
		ExperimentalUpgrade: experimentalUpgrade,
	}

	var f []*eskip.Filter

	if !preserveHeader {
		f = append(f, &eskip.Filter{
			Name: builtin.DropRequestHeaderName,
			Args: []interface{}{"Authorization"}})
	}

	if audit {
		f = append([]*eskip.Filter{&eskip.Filter{
			Name: logfilter.AuditLogName,
			Args: []interface{}{float64(auditBody)}}}, f...)
	}

	err := skipper.Run(o)
	if err != nil {
		log.Fatal(err)
	}
}
