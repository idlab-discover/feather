package storage

import (
	"archive/tar"
	"context"
	"fmt"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
	"io"
	"os"
	"path"
	"regexp"
)

func CleanName(name string) string {
	return regexp.MustCompile(":[0-9]{1,5}").ReplaceAllString(name, "")
}

func ensureDirRef(r ref.Ref) (ref.Ref, error) {
	if r.Scheme == "ocidir" {
		return r, nil
	}
	return ref.New(fmt.Sprintf("ocidir://%s", ImagePath(r.CommonName())))
}

func shouldPullImage(ctx context.Context, r ref.Ref) bool {
	// Check if tag is latest
	if r.Tag == "" || r.Tag == "latest" {
		return true
	}
	// Check if manifests can be retrieved
	_, err := regclient.New().ManifestGet(ctx, r)
	return err != nil
}

func extractTarToDir(reader io.Reader, root string) error {
	// Make root directory
	err := os.MkdirAll(root, 0755)
	if err != nil {
		return err
	}
	// Extract tar archive
	tarReader := tar.NewReader(reader)
	for header, err := tarReader.Next(); err == nil; header, err = tarReader.Next() {
		name := path.Join(root, header.Name)
		// Check type of entry
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.Mkdir(name, os.FileMode(header.Mode)); err != nil {
				if !os.IsExist(err) {
					return err
				}
			}
		case tar.TypeReg:
			file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown type: %b in %s", header.Typeflag, header.Name)
		}
	}
	return nil
}
