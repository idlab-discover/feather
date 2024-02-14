package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	ociv1ext "gitlab.ilabt.imec.be/fledge/service/pkg/oci/v1/ext"
	"path"
	"regexp"
)

func ImagesPath() string {
	return path.Join(RootPath(), "images")
}

func ImagePath(name string) string {
	name = regexp.MustCompile(":[0-9]{1,5}").ReplaceAllString(name, "")
	return path.Join(ImagesPath(), name)
}

//func ImagePull(ctx context.Context, name string) (ref.Ref, error) {
//	// Parse image source
//	src, err := ref.New(name)
//	if err != nil {
//		return ref.Ref{}, err
//	}
//	if src.Registry == "" {
//		return ref.Ref{}, fmt.Errorf("reference %s does not contain a valid registry", src.CommonName())
//	}
//	// Construct image target
//	tgt, err := ensureDirRef(src)
//	if err != nil {
//		return ref.Ref{}, err
//	}
//	// Check if image should be pulled
//	if !shouldPullImage(ctx, tgt) {
//		log.G(ctx).Infof("Image %s is present\n", tgt.CommonName())
//		return tgt, nil
//	}
//	// Copy image locally
//	client := regclient.New(regclient.WithDockerCreds())
//	imageOpts := []regclient.ImageOpts{
//		regclient.ImageWithPlatforms([]string{platforms.DefaultString()}),
//	}
//	if err = client.ImageCopy(ctx, src, tgt, imageOpts...); err != nil {
//		return ref.Ref{}, err
//	}
//	log.G(ctx).Infof("Pulled image %s to %s\n", src.CommonName(), tgt.CommonName())
//	return tgt, nil
//}

func ImageGetConfig(ctx context.Context, name string) (ociv1ext.Image, error) {
	// Parse image source
	src, err := ref.New(name)
	if err != nil {
		return ociv1ext.Image{}, err
	}
	if src.Registry == "" {
		return ociv1ext.Image{}, fmt.Errorf("reference %s does not contain a valid registry", src.CommonName())
	}
	// Retrieve manifest of the image
	rc := regclient.New(regclient.WithDockerCreds())
	return ImageGetConfigWithClient(rc, ctx, src)
}

func ImageGetConfigWithClient(rc *regclient.RegClient, ctx context.Context, r ref.Ref) (ociv1ext.Image, error) {
	// Retrieve manifest of the image
	manifestDesc, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return ociv1ext.Image{}, err
	}

	// Check if this manifest is supported
	manifestMediaType := manifestDesc.GetDescriptor().MediaType
	switch manifestMediaType {
	case types.MediaTypeDocker1Manifest, types.MediaTypeDocker1ManifestSigned, types.MediaTypeDocker2Manifest, types.MediaTypeOCI1Manifest:
	case types.MediaTypeDocker2ManifestList, types.MediaTypeOCI1ManifestList:
	default:
		return ociv1ext.Image{}, errors.Errorf("image %s has an unsupported media type %s", r.CommonName(), manifestMediaType)
	}

	// Find the specific manifest if the retrieved one is an index
	if manifestDesc.IsList() {
		// Find the manifest corresponding with this platform
		p := platform.Local()
		d, err := manifest.GetPlatformDesc(manifestDesc, &p)
		if err != nil {
			return ociv1ext.Image{}, err
		}
		rp := r
		rp.Digest = d.Digest.String()
		manifestDesc, err = rc.ManifestGet(ctx, rp)
		if err != nil {
			return ociv1ext.Image{}, err
		}
	}

	// Retrieve config from the manifest
	if img, ok := manifestDesc.(manifest.Imager); ok {
		configDesc, err := img.GetConfig()
		if err != nil {
			// In some manifests, no config is available. This is not an error
			return ociv1ext.Image{}, nil
		}
		configBlob, err := rc.BlobGet(ctx, r, configDesc)
		if err != nil {
			return ociv1ext.Image{}, err
		}
		var config ociv1ext.Image
		if err = json.NewDecoder(configBlob).Decode(&config); err != nil {
			return ociv1ext.Image{}, err
		}
		return config, nil
	}
	return ociv1ext.Image{}, fmt.Errorf("reference %s does not represent a valid image", r.CommonName())
}

func ImageGetLayers(ctx context.Context, name string) ([]types.Descriptor, error) {
	// Parse image source
	src, err := ref.New(name)
	if err != nil {
		return nil, err
	}
	if src.Registry == "" {
		return nil, fmt.Errorf("reference %s does not contain a valid registry", src.CommonName())
	}
	// Retrieve manifest of the image
	rc := regclient.New(regclient.WithDockerCreds())
	return ImageGetLayersWithClient(rc, ctx, src)
}

func ImageGetLayersWithClient(rc *regclient.RegClient, ctx context.Context, r ref.Ref) ([]types.Descriptor, error) {
	// Retrieve manifest of the image
	manifestDesc, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return nil, err
	}

	// Check if this manifest is supported
	manifestMediaType := manifestDesc.GetDescriptor().MediaType
	switch manifestMediaType {
	case types.MediaTypeDocker1Manifest, types.MediaTypeDocker1ManifestSigned, types.MediaTypeDocker2ImageConfig, types.MediaTypeOCI1Manifest:
	case types.MediaTypeDocker2ManifestList, types.MediaTypeOCI1ManifestList:
	default:
		return nil, errors.Errorf("image %s has an unsupported media type %s", r.CommonName(), manifestMediaType)
	}

	// Find the specific manifest if the retrieved one is an index
	if manifestDesc.IsList() {
		// Find the manifest corresponding with this platform
		p := platform.Local()
		d, err := manifest.GetPlatformDesc(manifestDesc, &p)
		if err != nil {
			return nil, err
		}
		rp := r
		rp.Digest = d.Digest.String()
		manifestDesc, err = rc.ManifestGet(ctx, rp)
		if err != nil {
			return nil, err
		}
	}

	// Retrieve config from the manifest
	if img, ok := manifestDesc.(manifest.Imager); ok {
		return img.GetLayers()
	}
	return nil, fmt.Errorf("reference %s does not represent a valid image", r.CommonName())
}
