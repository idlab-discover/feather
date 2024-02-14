package storage

import (
	"os"
	"path"
)

var rootPath string

func DefaultPath() string {
	if home, _ := os.UserHomeDir(); home != "" {
		return path.Join(home, ".fledge")
	}
	return "/var/lib/fledge"
}

func SetRootPath(path string) {
	rootPath = path
}

func RootPath() string {
	if rootPath == "" {
		return DefaultPath()
	}
	return rootPath
}
