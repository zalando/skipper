package main

import (
	"errors"
	"flag"
	"golang.org/x/crypto/ssh/terminal"
	"net/url"
	"os"
	"strings"
)

const (
	etcdUrlsFlag        = "etcd-urls"
	etcdStorageRootFlag = "etcd-storage-root"
	inlineRoutesFlag    = "routes"
	inlineIdsFlag       = "ids"

	defaultEtcdUrls        = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	defaultEtcdStorageRoot = "/skipper"

	etcdUrlsUsage        = "urls of nodes in an etcd cluster, storing route definitions"
	etcdStorageRootUsage = "path prefix for skipper related data in etcd"
	inlineRoutesUsage    = "inline routes in eskip format"
	inlineIdsUsage       = "comma separated route ids"
)

// used to prevent flag.FlagSet of printing errors in the wrong place
type noopWriter struct{}

func (w *noopWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

var invalidNumberOfArgs = errors.New("invalid number of args")

// parsing vars for flags:
var (
	etcdUrls        string
	etcdStorageRoot string
	inlineRoutes    string
	inlineRouteIds  string
)

var (
	isTest  = false
	nowrite = &noopWriter{}
	flags   *flag.FlagSet
)

func initFlags() {
	flags = &flag.FlagSet{Usage: func() {}}
	flags.SetOutput(nowrite)

	// the default value not used here, because it depends on the command
	flags.StringVar(&etcdUrls, etcdUrlsFlag, "", etcdUrlsUsage)
	flags.StringVar(&etcdStorageRoot, etcdStorageRootFlag, "", etcdStorageRootUsage)

	flags.StringVar(&inlineRoutes, inlineRoutesFlag, "", inlineRoutesUsage)
	flags.StringVar(&inlineRouteIds, inlineIdsFlag, "", inlineIdsUsage)
}

func init() {
	initFlags()
}

func processEtcdArgs(etcdUrls, etcdStorageRoot string) (*medium, error) {
	if etcdUrls == "" {
		etcdUrls = defaultEtcdUrls
	}

	if etcdStorageRoot == "" {
		etcdStorageRoot = defaultEtcdStorageRoot
	}

	surls := strings.Split(etcdUrls, ",")
	urls := make([]*url.URL, len(surls))
	for i, su := range surls {
		u, err := url.Parse(su)
		if err != nil {
			return nil, err
		}

		urls[i] = u
	}

	return &medium{
		typ:  etcd,
		urls: urls,
		path: etcdStorageRoot}, nil
}

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

func processStdin() (*medium, error) {

	// what can go wrong
	fdint := int(os.Stdin.Fd())

	if isTest || terminal.IsTerminal(fdint) {
		return nil, nil
	}

	return &medium{typ: stdin}, nil
}

func processArgs() ([]*medium, error) {
	err := flags.Parse(os.Args[2:])
	if err != nil {
		return nil, err
	}

	var media []*medium

	if etcdUrls != "" || etcdStorageRoot != "" {
		m, err := processEtcdArgs(etcdUrls, etcdStorageRoot)
		if err != nil {
			return nil, err
		}

		media = append(media, m)
	}

	if inlineRoutes != "" {
		media = append(media, &medium{
			typ:   inline,
			eskip: inlineRoutes})
	}

	if inlineRouteIds != "" {
		media = append(media, &medium{
			typ: inlineIds,
			ids: strings.Split(inlineRouteIds, ",")})
	}

	fileArg, err := processFileArg()
	if err != nil {
		return nil, err
	}

	if fileArg != nil {
		media = append(media, fileArg)
	}

	stdinArg, err := processStdin()
	if err != nil {
		return nil, err
	}

	if stdinArg != nil {
		media = append(media, stdinArg)
	}

	return media, nil
}
