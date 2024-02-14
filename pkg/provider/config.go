package provider

import (
	"gitlab.ilabt.imec.be/fledge/service/pkg/config"
)

// Defaults for the provider
var defaultConfig = Config{
	Default: BackendContainerd,
	Enabled: []string{BackendContainerd},
}

// Config contains a provider virtual-kubelet's configurable parameters.
type Config struct { //nolint:golint
	config.Config
	Default string   `json:"default,omitempty"`
	Enabled []string `json:"enabled,omitempty"`
}
