package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"github.com/virtual-kubelet/virtual-kubelet/trace/opencensus"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/commands/providers"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/commands/root"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/commands/version"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/provider"
)

var (
	buildVersion = "N/A"
	buildTime    = "N/A"
)

func main() {
	/* Boilerplate from https://github.com/virtual-kubelet/virtual-kubelet/blob/master/cmd/virtual-kubelet/main.go */
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()

	log.L = logruslogger.FromLogrus(logrus.NewEntry(logrus.StandardLogger()))
	trace.T = opencensus.Adapter{}

	var opts root.Opts
	optsErr := root.SetDefaultOpts(&opts)
	// opts.Version = strings.Join([]string{k8sVersion, "vk", buildVersion}, "-")

	s := provider.NewStore()
	//registerMock(ctx, s)
	registerBackend(ctx, s) // FLEDGE
	//registerBroker(ctx, s)  // FLEDGE

	rootCmd := root.NewCommand(ctx, filepath.Base(os.Args[0]), s, opts)
	rootCmd.AddCommand(version.NewCommand(buildVersion, buildTime), providers.NewCommand(s))
	preRun := rootCmd.PreRunE

	var logLevel string
	rootCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if optsErr != nil {
			return optsErr
		}
		if preRun != nil {
			return preRun(cmd, args)
		}
		return nil
	}

	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", `set the log level, e.g. "debug", "info", "warn", "error"`)

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if logLevel != "" {
			lvl, err := logrus.ParseLevel(logLevel)
			if err != nil {
				return errors.Wrap(err, "could not parse log level")
			}
			logrus.SetLevel(lvl)
		}
		return nil
	}

	patchCmd(ctx, rootCmd, s, opts) // FLEDGE
	// Prometheus
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.ListenAndServe(":2112", nil)
	}()

	if err := rootCmd.Execute(); err != nil && errors.Cause(err) != context.Canceled {
		log.G(ctx).Fatal(err)
	}
}
