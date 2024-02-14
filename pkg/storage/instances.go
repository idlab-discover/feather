package storage

import (
	"path"
	"regexp"
)

func InstancesPath() string {
	return path.Join(RootPath(), "instances")
}

func InstancePath(name string) string {
	name = regexp.MustCompile(":[0-9]{1,5}").ReplaceAllString(name, "")
	return path.Join(InstancesPath(), name)
}
