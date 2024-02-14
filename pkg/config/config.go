package config

import (
	"context"
	"encoding/json"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"gitlab.ilabt.imec.be/fledge/service/pkg/system"
	"gopkg.in/validator.v2"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type Config struct {
	// ServerCertPath is the path to the certificate to secure the kubelet API.
	ServerCertPath string `json:"serverCertPath" env:"SERVER_CERT_PATH" validate:"nonzero"`

	// ServerKeyPath is the path to the private key to sign the kubelet API.
	ServerKeyPath string `json:"serverKeyPath" env:"SERVER_KEY_PATH" validate:"nonzero"`

	// NodeInternalIP is the desired Node internal IP.
	NodeInternalIP net.IP `json:"nodeInternalIP" env:"NODE_INTERNAL_IP"`

	// NodeExternalIP is the desired Node external IP.
	NodeExternalIP net.IP `json:"nodeExternalIP" env:"NODE_EXTERNAL_IP"`

	//// NodeInternalIface is interface's name whose address to use for Node internal IP.
	//NodeInternalIface string
	//
	//// NodeExternalIface is the interface's name whose address to use for Node external IP.
	//NodeExternalIface string

	// KubernetesURL is the value to set for the KUBERNETES_SERVICE_* Pod env vars.
	KubernetesURL string `json:"kubernetesURL" env:"KUBERNETES_URL"`

	// ListenAddress is the address to bind for serving requests from the Kubernetes API server.
	ListenAddress string `json:"listenAddress" env:"LISTEN_ADDRESS"`

	// NodeName identifies the Node in the cluster.
	NodeName string `json:"nodeName" env:"NODE_NAME"`

	// DisableTaint disables fledge default taint.
	DisableTaint bool `json:"disableTaint" env:"DISABLE_TAINT"`

	// MetricsAddress is the address to bind for serving metrics.
	MetricsAddress string `json:"metricsAddress" env:"METRICS_ADDRESS"`

	// PodSyncWorkers is the number of workers that handle Pod events.
	PodSyncWorkers int `json:"podSyncWorkers" env:"POD_SYNC_WORKERS"`
}

func LoadConfig(ctx context.Context, path string) (*Config, error) {
	log.G(ctx).Debugf("Loading config from %s..\n", path)
	cfg := &Config{}

	// Open file
	file, err := os.Open(path)
	defer file.Close()

	// Fallback to an empty json if the file does not exist
	var reader io.Reader
	if err == nil {
		reader = file
	} else if os.IsNotExist(err) {
		log.G(ctx).Debugf("'%s' does not exist\n", path)
		reader = strings.NewReader("{}")
	} else if err != nil {
		return nil, err
	}

	// Parse file as JSON
	if err = json.NewDecoder(reader).Decode(&cfg); err != nil {
		return nil, err
	}

	// Override config with env vars
	t := reflect.TypeOf(*cfg)
	v := reflect.ValueOf(cfg)
	for i := 0; i < t.NumField(); i++ {
		tf := t.Field(i)
		vf := v.Elem().Field(i)
		if tag := tf.Tag.Get("env"); tag != "" {
			if val := os.Getenv("FLEDGE_" + tag); val != "" {
				if vf.Kind() == reflect.Int {
					intVal, err := strconv.Atoi(val)
					if err != nil {
						return nil, err
					}
					vf.SetInt(int64(intVal))
				} else if vf.Kind() == reflect.String {
					vf.SetString(val)
				}
			}
		}
	}
	// Validate configuration
	if err := validator.Validate(cfg); err != nil {
		return nil, err
	}
	log.G(ctx).Debugf("Config is valid %+v\n", cfg)
	// Set defaults
	if cfg.NodeInternalIP == nil {
		cfg.NodeInternalIP = system.InternalIP()
	}
	if cfg.NodeExternalIP == nil {
		cfg.NodeExternalIP = system.ExternalIP()
	}
	if cfg.NodeName == "" {
		cfg.NodeName = system.HostName()
	}
	if cfg.PodSyncWorkers == 0 {
		cfg.PodSyncWorkers = 10
	}
	return cfg, nil
}
