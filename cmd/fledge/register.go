package main

import (
	"context"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/provider"
	"gitlab.ilabt.imec.be/fledge/service/cmd/fledge/internal/provider/mock"
	"gitlab.ilabt.imec.be/fledge/service/pkg/fledge"
	backend "gitlab.ilabt.imec.be/fledge/service/pkg/provider"
)

func registerMock(ctx context.Context, s *provider.Store) {
	/* #nosec */
	s.Register("mock", func(cfg provider.InitConfig) (provider.Provider, error) { //nolint:errcheck
		return mock.NewMockProvider(
			cfg.ConfigPath,
			cfg.NodeName,
			cfg.OperatingSystem,
			cfg.InternalIP,
			cfg.DaemonPort,
		)
	})
}

func registerBackend(ctx context.Context, s *provider.Store) {
	s.Register("backend", func(cfg provider.InitConfig) (provider.Provider, error) {
		return backend.NewProvider(
			ctx,
			cfg.ConfigPath,
			cfg.NodeName,
			cfg.OperatingSystem,
			cfg.ResourceManager,
			cfg.InternalIP,
			cfg.DaemonPort,
		)
	})
}

func registerBroker(ctx context.Context, s *provider.Store) {
	s.Register("broker", func(cfg provider.InitConfig) (provider.Provider, error) {
		return fledge.NewBrokerProvider(
			cfg.ConfigPath,
			cfg.NodeName,
			cfg.OperatingSystem,
			cfg.InternalIP,
			cfg.DaemonPort,
		)
	})
}
