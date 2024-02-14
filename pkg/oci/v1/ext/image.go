package ext

import ociv1 "github.com/regclient/regclient/types/oci/v1"

type Image struct {
	Backend    string `json:"fledge.backend,omitempty"`
	Hypervisor string `json:"fledge.hypervisor,omitempty"`
	ociv1.Image
}
