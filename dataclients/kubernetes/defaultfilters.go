package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
)

type filterSet struct {
	text    string
	filters []*eskip.Filter
	parsed  bool
	err     error
}

type defaultFilters map[definitions.ResourceID]*filterSet

func readDefaultFilters(dir string) (defaultFilters, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	filters := make(defaultFilters)
	for _, f := range files {
		r := strings.Split(f.Name(), ".") // format: {service}.{namespace}
		info, err := f.Info()
		if len(r) != 2 || !(f.Type().IsRegular() || f.Type()&os.ModeSymlink != 0) || info.Size() > maxFileSize {
			log.WithError(err).WithField("file", f.Name()).Debug("incompatible file")
			continue
		}

		file := filepath.Join(dir, f.Name())
		config, err := os.ReadFile(file)
		if err != nil {
			log.WithError(err).WithField("file", file).Debug("could not read file")
			continue
		}

		filters[definitions.ResourceID{Name: r[0], Namespace: r[1]}] = &filterSet{text: string(config)}
	}

	return filters, nil
}

func (fs *filterSet) parse() {
	if fs.parsed {
		return
	}

	fs.filters, fs.err = eskip.ParseFilters(fs.text)
	if fs.err != nil {
		fs.err = fmt.Errorf("[eskip] default filters: %v", fs.err)
	}

	fs.parsed = true
}

func (df defaultFilters) get(serviceID definitions.ResourceID) ([]*eskip.Filter, error) {
	fs, ok := df[serviceID]
	if !ok {
		return nil, nil
	}

	fs.parse()
	if fs.err != nil {
		return nil, fs.err
	}

	f := make([]*eskip.Filter, len(fs.filters))
	copy(f, fs.filters)
	return f, nil
}

func (df defaultFilters) getNamed(namespace, serviceName string) ([]*eskip.Filter, error) {
	return df.get(definitions.ResourceID{Namespace: namespace, Name: serviceName})
}
