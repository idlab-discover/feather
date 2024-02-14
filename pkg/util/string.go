package util

import "regexp"

var reSpace = regexp.MustCompile(`\s+`)

func RemoveSpace(s string) string {
	return reSpace.ReplaceAllString(s, "")
}
