package main

import (
	"context"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/commands/root"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/provider"
	"gitlab.ilabt.imec.be/fledge/service/pkg/config"
	"gitlab.ilabt.imec.be/fledge/service/pkg/storage"
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// / Patch virtual-kubelet without modifying the sources too much
func patchCmd(ctx context.Context, rootCmd *cobra.Command, s *provider.Store, c root.Opts) {
	var (
		configPath  string
		storagePath string
	)
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "default.json", "set the config path")
	rootCmd.PersistentFlags().StringVarP(&storagePath, "storage", "s", storage.DefaultPath(), "Root directory used by FLEDGE")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if configPath != "" {
			cfg, err := config.LoadConfig(ctx, configPath)
			if err != nil {
				return errors.Wrap(err, "could not parse config")
			}
			// Patch options with config values
			patchOpts(cfg, cmd, c)
		}
		if storagePath == "" {
			storagePath = storage.DefaultPath()
		}
		storage.SetRootPath(storagePath)
		return nil
	}
}

func patchOpts(cfg *config.Config, cmd *cobra.Command, c root.Opts) {
	// Set default commandline arguments
	patchOpt(cmd.Flags(), "nodename", cfg.NodeName)
	patchOpt(cmd.Flags(), "os", runtime.GOOS)
	patchOpt(cmd.Flags(), "provider", "backend")
	patchOpt(cmd.Flags(), "provider-config", "backend.json")
	patchOpt(cmd.Flags(), "metrics-addr", cfg.MetricsAddress)
	patchOpt(cmd.Flags(), "disable-taint", strconv.FormatBool(cfg.DisableTaint))
	patchOpt(cmd.Flags(), "pod-sync-workers", strconv.FormatInt(int64(cfg.PodSyncWorkers), 10))
	patchOpt(cmd.Flags(), "enable-node-lease", strconv.FormatBool(true))
	patchOpt(cmd.Flags(), "metrics-addr", cfg.NodeInternalIP.String())

	// Set kubernetes version
	k8sVersion, _ := util.ReadDepVersion("k8s.io/api")
	k8sVersion = regexp.MustCompile("^v0").ReplaceAllString(k8sVersion, "v1")
	c.Version = strings.Join([]string{k8sVersion, "fledge", buildVersion}, "-")

	// Populate apiserver options
	os.Setenv("APISERVER_CERT_LOCATION", cfg.ServerCertPath)
	os.Setenv("APISERVER_KEY_LOCATION", cfg.ServerKeyPath)
	os.Setenv("APISERVER_CA_CERT_LOCATION", cfg.ServerCertPath)

	// Populate vkubelet options
	os.Setenv("VKUBELET_POD_IP", cfg.NodeInternalIP.String())
}

func patchOpt(flags *flag.FlagSet, name string, value string) {
	f := flags.Lookup(name)
	if !f.Changed {
		f.Value.Set(value)
	}
}
