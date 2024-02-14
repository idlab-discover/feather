package hypervisor

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type Volume struct {
	Path    string `yaml:"-"`
	Format  string `yaml:"format,omitempty"` // raw|qcow2|...
	AioType string `yaml:"aio,omitempty"`    // native|threads
	Cache   string `yaml:"cache,omitempty"`  // none|unsafe|writethrough...
}

// ParseVolumes parses --volume strings that are of following format:
// --volume {volumePath}[:{options}]
// Example: --volume /path/to/myvolume.img:format=raw:aio=native
func ParseVolumes(volumeStrings []string) ([]Volume, error) {
	res := []Volume{}
	if volumeStrings == nil {
		return res, nil
	}
	for _, volumeStr := range volumeStrings {
		if v, err := parseVolume(volumeStr); err == nil {
			res = append(res, *v)
		} else {
			return res, err
		}

	}
	return res, nil
}

func parseVolume(volumeStr string) (*Volume, error) {
	v := &Volume{
		Path:    "",
		Format:  "raw",
		AioType: "native",
		Cache:   "none",
	}

	for idx, part := range strings.Split(volumeStr, ":") {
		if idx == 0 { // Volume path
			if path, err := filepath.Abs(part); err == nil {
				v.Path = path
				continue
			} else {
				return nil, err
			}
		}

		// Volume settings
		if !strings.Contains(part, "=") {
			return nil, fmt.Errorf("Please use '=' for assignment of volume settings. Example: --volume /vol.img:format=raw")
		}

		keyVal := strings.SplitN(part, "=", 2)
		keyVal[0] = strings.ToLower(keyVal[0])
		if keyVal[0] == "format" {
			v.Format = keyVal[1]
		} else if keyVal[0] == "aio" {
			v.AioType = keyVal[1]
		} else if keyVal[0] == "cache" {
			v.Cache = keyVal[1]
		} else {
			return nil, fmt.Errorf("Unknown volume setting: '%s'", keyVal[0])
		}

	}
	return v, nil
}

func (v *Volume) PersistMetadata() error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s.yaml", v.Path)
	return ioutil.WriteFile(path, []byte(data), 0644)
}
