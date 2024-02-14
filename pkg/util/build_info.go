package util

import (
	"errors"
	"fmt"
	"runtime/debug"
)

func ReadDepVersion(path string) (string, error) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("unable to read build info")
	}
	for _, dep := range bi.Deps {
		if dep.Path == path {
			return dep.Version, nil
		}
	}
	return "", errors.New(fmt.Sprintf("no such package %s", path))
}
