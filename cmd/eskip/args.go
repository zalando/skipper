package main

import (
	"errors"
	"flag"
	"net/url"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

const (
	etcdUrlsFlag            = "etcd-urls"
	etcdPrefixFlag          = "etcd-prefix"
	etcdOAuthTokenFlag      = "etcd-oauth-token"
	innkeeperUrlFlag        = "innkeeper-url"
	oauthTokenFlag          = "oauth-token"
	inlineRoutesFlag        = "routes"
	inlineIdsFlag           = "ids"
	insecureFlag            = "insecure"
	prependFiltersFlag      = "prepend"
	prependFileFlag         = "prepend-file"
	appendFiltersFlag       = "append"
	appendFileFlag          = "append-file"
	prettyFlag              = "pretty"
	indentStrFlag           = "indent"
	jsonFlag                = "json"
	kubernetesNameFlag      = "name"
	kubernetesNamespaceFlag = "namespace"
	hostnameFlag            = "hostname"

	defaultEtcdUrls     = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	defaultEtcdPrefix   = "/skipper"
	defaultInnkeeperUrl = "http://127.0.0.1:8080"
)

// used to prevent flag.FlagSet of printing errors in the wrong place
type noopWriter struct{}

func (w *noopWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

var (
	invalidNumberOfArgs = errors.New("invalid number of args")
	missingOAuthToken   = errors.New("missing OAuth token")
	invalidIndentStr    = errors.New("invalid indent. Must match regexp \\s")
)

// parsing vars for flags:
var (
	etcdUrls               string
	etcdPrefix             string
	innkeeperUrl           string
	oauthToken             string
	etcdOAuthToken         string
	inlineRoutes           string
	inlineRouteIds         string
	insecure               bool
	prependFiltersArg      string
	prependFileArg         string
	appendFiltersArg       string
	appendFileArg          string
	pretty                 bool
	indentStr              string
	printJson              bool
	kubernetesNameArg      string
	kubernetesNamespaceArg string
	hostname               string
)

var (
	// used to prevent automatic stdin detection during tests:
	isTest = false

	nowrite = &noopWriter{}
	flags   *flag.FlagSet
)

func initFlags() {
	flags = &flag.FlagSet{Usage: func() {}}
	flags.SetOutput(nowrite)

	// the default value not used here, because it depends on the command
	flags.StringVar(&etcdUrls, etcdUrlsFlag, "", etcdUrlsUsage)
	flags.StringVar(&etcdPrefix, etcdPrefixFlag, "", etcdPrefixUsage)
	flags.StringVar(&etcdOAuthToken, etcdOAuthTokenFlag, "", etcdOAuthTokenUsage)

	flags.StringVar(&innkeeperUrl, innkeeperUrlFlag, "", innkeeperUrlUsage)
	flags.StringVar(&oauthToken, oauthTokenFlag, "", oauthTokenUsage)

	flags.StringVar(&inlineRoutes, inlineRoutesFlag, "", inlineRoutesUsage)
	flags.StringVar(&inlineRouteIds, inlineIdsFlag, "", inlineIdsUsage)

	flags.BoolVar(&insecure, insecureFlag, false, insecureUsage)

	flags.StringVar(&prependFiltersArg, prependFiltersFlag, "", prependFiltersUsage)
	flags.StringVar(&prependFileArg, prependFileFlag, "", prependFileUsage)
	flags.StringVar(&appendFiltersArg, appendFiltersFlag, "", appendFiltersUsage)
	flags.StringVar(&appendFileArg, appendFileFlag, "", appendFileUsage)

	flags.BoolVar(&pretty, prettyFlag, false, prettyUsage)
	flags.StringVar(&indentStr, indentStrFlag, "  ", indentStrUsage)
	flags.BoolVar(&printJson, jsonFlag, false, jsonUsage)

	flags.StringVar(&kubernetesNameArg, kubernetesNameFlag, "", kubernetesNameUsage)
	flags.StringVar(&kubernetesNamespaceArg, kubernetesNamespaceFlag, "", kubernetesNamespaceUsage)
	flags.StringVar(&hostname, hostnameFlag, "", hostnameUsage)
}

func init() {
	initFlags()
}

func urlsToStrings(urls []*url.URL) []string {
	surls := make([]string, len(urls))
	for i, u := range urls {
		surls[i] = u.String()
	}

	return surls
}

func stringsToUrls(strs ...string) ([]*url.URL, error) {
	urls := make([]*url.URL, len(strs))
	for i, su := range strs {
		u, err := url.Parse(su)
		if err != nil {
			return nil, err
		}

		urls[i] = u
	}

	return urls, nil
}

// returns etcd type medium if any of '-etcd-urls' or '-etcd-prefix'
// are defined.
func processEtcdArgs(etcdUrls, etcdPrefix, oauthToken string) (*medium, error) {
	if etcdUrls == "" && etcdPrefix == "" {
		return nil, nil
	}

	if etcdUrls == "" {
		etcdUrls = defaultEtcdUrls
	}

	if etcdPrefix == "" {
		etcdPrefix = defaultEtcdPrefix
	}

	surls := strings.Split(etcdUrls, ",")
	urls, err := stringsToUrls(surls...)
	if err != nil {
		return nil, err
	}

	return &medium{
		typ:        etcd,
		urls:       urls,
		path:       etcdPrefix,
		oauthToken: oauthToken}, nil
}

func processInnkeeperArgs(innkeeperUrl, oauthToken string) (*medium, error) {
	if innkeeperUrl == "" && oauthToken == "" {
		return nil, nil
	}

	if oauthToken == "" {
		return nil, missingOAuthToken
	}

	if innkeeperUrl == "" {
		innkeeperUrl = defaultInnkeeperUrl
	}

	urls, err := stringsToUrls(innkeeperUrl)
	if err != nil {
		return nil, err
	}

	return &medium{
		typ:        innkeeper,
		urls:       urls,
		oauthToken: oauthToken}, nil
}

// returns file type medium if a positional parameter is defined.
func processFileArg() (*medium, error) {
	nonFlagArgs := flags.Args()
	if len(nonFlagArgs) > 1 {
		return nil, invalidNumberOfArgs
	}

	if len(nonFlagArgs) == 0 {
		return nil, nil
	}

	return &medium{
		typ:  file,
		path: nonFlagArgs[0]}, nil
}

// if pretty print then check that indent matches pattern
func processIndentStr() error {
	if pretty && !(regexp.MustCompile(`^[\s]*$`).MatchString(indentStr)) {
		return invalidIndentStr
	}
	return nil

}

// returns stdin type medium if stdin is not TTY.
func processStdin() *medium {

	// what can go wrong
	fdint := int(os.Stdin.Fd())

	if isTest || terminal.IsTerminal(fdint) {
		return nil
	}

	return &medium{typ: stdin}
}

func processPatchArgs(pfilters, pfile, afilters, afile string) []*medium {
	var media []*medium

	if pfilters != "" {
		media = append(media, &medium{typ: patchPrepend, patchFilters: pfilters})
	}

	if afilters != "" {
		media = append(media, &medium{typ: patchAppend, patchFilters: afilters})
	}

	if pfile != "" {
		media = append(media, &medium{typ: patchPrependFile, patchFile: pfile})
	}

	if afile != "" {
		media = append(media, &medium{typ: patchAppendFile, patchFile: afile})
	}

	return media
}

func processKubernetesArgs(name, namespace, hostname string) []*medium {
	var media []*medium
	if name != "" {
		media = append(media, &medium{typ: kubernetesName, kubernetesName: name})
	}

	if namespace != "" {
		media = append(media, &medium{typ: kubernetesNamespace, kubernetesNamespace: namespace})
	}

	hns := strings.Split(hostname, ",")
	if len(hns) > 1 || hns[0] != "" {
		media = append(media, &medium{typ: hostnames, hostnames: hns})
	}

	return media
}

// returns media detected from the executing command.
func processArgs() ([]*medium, error) {
	err := flags.Parse(os.Args[2:])
	if err != nil {
		return nil, err
	}

	var media []*medium
	innkeeperArg, err := processInnkeeperArgs(innkeeperUrl, oauthToken)
	if err != nil {
		return nil, err
	}

	if innkeeperArg != nil {
		media = append(media, innkeeperArg)
	}

	etcdArg, err := processEtcdArgs(etcdUrls, etcdPrefix, etcdOAuthToken)
	if err != nil {
		return nil, err
	}

	if etcdArg != nil {
		media = append(media, etcdArg)
	}

	if inlineRoutes != "" {
		media = append(media, &medium{
			typ:   inline,
			eskip: inlineRoutes,
		})
	}

	if inlineRouteIds != "" {
		media = append(media, &medium{
			typ: inlineIds,
			ids: strings.Split(inlineRouteIds, ","),
		})
	}

	fileArg, err := processFileArg()
	if err != nil {
		return nil, err
	}

	err = processIndentStr()
	if err != nil {
		return nil, err
	}

	if fileArg != nil {
		media = append(media, fileArg)
	} else {
		stdinArg := processStdin()
		if stdinArg != nil {
			media = append(media, stdinArg)
		}
	}

	patchMedia := processPatchArgs(
		prependFiltersArg,
		prependFileArg,
		appendFiltersArg,
		appendFileArg,
	)

	media = append(media, patchMedia...)

	kube := processKubernetesArgs(
		kubernetesNameArg,
		kubernetesNamespaceArg,
		hostname,
	)

	media = append(media, kube...)
	return media, nil
}
