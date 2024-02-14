package storage

import (
	"path"
	"regexp"
)

func VolumesPath() string {
	return path.Join(RootPath(), "volumes")
}

func VolumePath(name string) string {
	name = regexp.MustCompile(":[0-9]{1,5}").ReplaceAllString(name, "")
	return path.Join(VolumesPath(), name)
}
