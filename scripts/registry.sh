#!/bin/sh
# OCI Distribution spec examples

TOKEN=$(curl -s "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/ubuntu:pull" | jq -r .token)
http get https://registry.hub.docker.com/v2/library/ubuntu/ "Authorization:Bearer $TOKEN"

# list tags
http get https://registry.hub.docker.com/v2/library/ubuntu/tags/list "Authorization:Bearer $TOKEN"

# list manifests
http get https://registry.hub.docker.com/v2/library/ubuntu/manifests/23.04 "Authorization:Bearer $TOKEN" "Accept:application/vnd.oci.image.index.v1+json"

# get a manifest by its digest
http get https://registry.hub.docker.com/v2/library/ubuntu/manifests/sha256:b2175cd4cfdd5cdb1740b0e6ec6bbb4ea4892801c0ad5101a81f694152b6c559 "Authorization:Bearer $TOKEN" "Accept:application/vnd.oci.image.manifest.v1+json"

# get config by its digest
http --follow get https://registry.hub.docker.com/v2/library/ubuntu/blobs/sha256:74f2314a03de34a0a2d552b805411fc9553a02ea71c1291b815b2f645f565683 "Authorization:Bearer $TOKEN" "Accept:application/vnd.oci.imag
e.config.v1+json"

# get layer by its digest
http --follow get https://registry.hub.docker.com/v2/library/ubuntu/blobs/sha256:76769433fd8a87dd77a6ce33db12156b1ea8dad3da3a95e7c9c36a47ec17b24c "Authorization:Bearer $TOKEN" "Accept:application/vnd.oci.image.layer.v1+json"

