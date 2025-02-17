package gvm

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"

	"github.com/andrewkroh/gvm/common"
)

var reGostoreVersion = regexp.MustCompile(`go(.*)\.(.*)-(.*)\..*`)

func (m *Manager) installBinary(version *GoVersion) (string, error) {
	godir := m.versionDir(version)

	tmp, err := ioutil.TempDir("", godir)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	extension := "tar.gz"
	if m.GOOS == "windows" {
		extension = "zip"
	}

	goURL := fmt.Sprintf("%s/go%v.%v-%v.%v", m.GoStorageHome, version, m.GOOS, m.GOARCH, extension)
	path, err := common.DownloadFile(goURL, tmp, m.HTTPTimeout)
	if err != nil {
		return "", fmt.Errorf("failed downloading from %v: %w", goURL, err)
	}

	return extractTo(m.VersionGoROOT(version), path)
}

func (m *Manager) AvailableBinaries() ([]*GoVersion, error) {
	home, goos, goarch := m.GoStorageHome, m.GOOS, m.GOARCH

	versions := map[string]struct{}{}
	err := m.iterXMLDirListing(home, func(name string) bool {
		matches := reGostoreVersion.FindStringSubmatch(name)
		if len(matches) < 4 {
			return true
		}

		matches = matches[1:]
		if matches[1] != goos || matches[2] != goarch {
			return true
		}

		versions[matches[0]] = struct{}{}
		return true
	})
	if err != nil {
		return nil, err
	}

	list := make([]*GoVersion, 0, len(versions))
	for version := range versions {
		ver, err := ParseVersion(version)
		if err != nil {
			continue
		}

		list = append(list, ver)
	}

	sortVersions(list)
	return list, nil
}

func (m *Manager) iterXMLDirListing(home string, fn func(entry string) bool) error {
	marker := ""
	client := &http.Client{
		Timeout: m.HTTPTimeout,
	}

	for {
		type contents struct {
			Key string
		}

		listing := struct {
			IsTruncated bool
			NextMarker  string
			Contents    []contents
		}{}

		req, err := http.NewRequest("GET", home, nil)
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Add("marker", marker)
		req.URL.RawQuery = q.Encode()

		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("listing failed with http status %v", resp.StatusCode)
		}

		dec := xml.NewDecoder(resp.Body)
		if err := dec.Decode(&listing); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		for i := range listing.Contents {
			cont := fn(listing.Contents[i].Key)
			if !cont {
				return nil
			}
		}

		next := listing.NextMarker
		if next == "" {
			return nil
		}
		marker = next
	}
}
