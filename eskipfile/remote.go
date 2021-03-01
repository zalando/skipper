package eskipfile

import (
	"errors"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

type remoteEskipFile struct {
	preloaded       bool
	remotePath      string
	localPath       string
	eskipFileClient *WatchClient
	threshold       int
	verbose         bool
}

// RemoteWatch creates a route configuration client with (remote) file watching. Watch doesn't follow file system nodes,
// it always reads (or re-downloads) from the file identified by the initially provided file name.
func RemoteWatch(rf string, th int, v bool) (routing.DataClient, error) {
	if !isFileRemote(rf) {
		return Watch(rf), nil
	}

	tempFilename, err := ioutil.TempFile("", "routes")

	if err != nil {
		return nil, err
	}

	dataClient := &remoteEskipFile{
		remotePath: rf,
		localPath:  tempFilename.Name(),
		threshold:  th,
		verbose:    v,
	}

	err = dataClient.DownloadRemoteFile()

	if err != nil {
		return nil, err
	}

	dataClient.eskipFileClient = Watch(tempFilename.Name())

	dataClient.preloaded = true

	return dataClient, nil
}

// LoadAll returns the parsed route definitions found in the file.
func (client *remoteEskipFile) LoadAll() ([]*eskip.Route, error) {

	var err error = nil

	if client.preloaded {
		client.preloaded = false
	} else {
		err = client.DownloadRemoteFile()
	}

	if err != nil {
		log.Errorf("LoadAll from remote %s failed. Continue using the last loaded routes", client.remotePath)
		return nil, err
	}

	if client.verbose {
		log.Debugf("New routes file %s was downloaded", client.remotePath)
	}

	return client.eskipFileClient.LoadAll()
}

// LoadUpdate returns differential updates when a remote file has changed.
func (client *remoteEskipFile) LoadUpdate() ([]*eskip.Route, []string, error) {

	err := client.DownloadRemoteFile()

	if err != nil {
		log.Errorf("LoadUpdate from remote %s failed. Trying to LoadAll", client.remotePath)
		return nil, nil, err
	}

	newRoutes, deletedRoutes, err := client.eskipFileClient.LoadUpdate()
	if err == nil {
		if client.verbose {
			log.Infof("New routes were loaded. New: %d; deleted: %d", len(newRoutes), len(deletedRoutes))

			if client.threshold >= 0 {
				if len(newRoutes) > client.threshold || len(deletedRoutes) > client.threshold {
					log.Warnf("Significant amount of routes was updated. New: %d; deleted: %d", len(newRoutes), len(deletedRoutes))
				}
			}
		}
	} else {
		log.Errorf("RemoteEskipFile LoadUpdate %s failed. Skipper continues to serve the last successfully updated routes. Error: %s",
			client.remotePath, err)
	}

	return newRoutes, deletedRoutes, err
}

func isFileRemote(remotePath string) bool {
	return strings.HasPrefix(remotePath, "http://") || strings.HasPrefix(remotePath, "https://")
}

func (client *remoteEskipFile) DownloadRemoteFile() error {

	data, err := client.GetRemoteData()
	if err != nil {
		return err
	}
	defer data.Close()

	out, err := os.OpenFile(client.localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, data); err != nil {
		_ = out.Close()
		return err
	}

	return out.Close()
}

func (client *remoteEskipFile) GetRemoteData() (io.ReadCloser, error) {

	resp, err := http.Get(client.remotePath)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("download file failed")
	}

	return resp.Body, nil
}
