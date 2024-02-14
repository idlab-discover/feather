package provider

import (
	"context"
	"gitlab.ilabt.imec.be/fledge/service/pkg/manager"
	"time"
)

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	corev1 "k8s.io/api/core/v1"
	"os"
)

const (
	// Values used in tracing as attribute keys.
	namespaceKey     = "namespace"
	nameKey          = "name"
	containerNameKey = "containerName"
)

// Provider implements the virtual-kubelet provider interface and forwards calls to runtimes.
type Provider struct {
	nodeName           string
	operatingSystem    string
	resourceManager    *manager.ResourceManager
	internalIP         string
	daemonEndpointPort int32
	config             Config
	startTime          time.Time
	backends           map[string]Backend
	pods               map[string]*corev1.Pod
	instances          map[string]*Instance
}

// NewProviderConfig creates a new Provider.
func NewProviderConfig(ctx context.Context, config Config, nodeName, operatingSystem string, resourceManager *manager.ResourceManager, internalIP string, daemonEndpointPort int32) (*Provider, error) {
	// set defaults
	if config.Default == "" {
		config.Default = defaultConfig.Default
	}
	if len(config.Enabled) == 0 {
		config.Enabled = defaultConfig.Enabled
	}
	// setup backend
	backends := map[string]Backend{}
	var err error
	for _, e := range config.Enabled {
		switch e {
		case BackendContainerd:
			if backends[e], err = NewContainerdBackend(ctx, config); err != nil {
				return nil, err
			}
		case BackendOsv:
			if backends[e], err = NewOSvBackend(ctx, config); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New(fmt.Sprintf("backend '%s' is not supported\n", e))
		}
	}

	// setup provider
	provider := Provider{
		nodeName:           nodeName,
		operatingSystem:    operatingSystem,
		resourceManager:    resourceManager,
		internalIP:         internalIP,
		daemonEndpointPort: daemonEndpointPort,
		pods:               map[string]*corev1.Pod{},
		config:             config,
		startTime:          time.Now(),
		backends:           backends,
		instances:          map[string]*Instance{},
	}
	return &provider, nil
}

// NewProvider creates a new Provider, which implements the PodNotifier interface
func NewProvider(ctx context.Context, providerConfig, nodeName, operatingSystem string, resourceManager *manager.ResourceManager, internalIP string, daemonEndpointPort int32) (*Provider, error) {
	cfg, err := loadConfig(providerConfig)
	if err != nil {
		return nil, err
	}

	return NewProviderConfig(ctx, cfg, nodeName, operatingSystem, resourceManager, internalIP, daemonEndpointPort)
}

// loadConfig loads the given json configuration files.
func loadConfig(providerConfig string) (cfg Config, err error) {
	data, err := os.ReadFile(providerConfig)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, err
	}
	if cfg.Default == "" {
		cfg.Default = defaultConfig.Default
	}
	if len(cfg.Enabled) == 0 {
		cfg.Enabled = defaultConfig.Enabled
	}
	return cfg, nil
}

// TODO: Implement NodeChanged for performance reasons

// Ensure interface is implemented
var _ nodeutil.Provider = (*Provider)(nil)
