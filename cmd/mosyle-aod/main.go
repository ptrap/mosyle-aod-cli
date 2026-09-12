package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ptrap/mosyle-aod-cli/internal/aod"
	"github.com/ptrap/mosyle-aod-cli/internal/cli"
	"github.com/ptrap/mosyle-aod-cli/internal/localauth"
)

var version = "dev"

type backend struct{ s aod.System }

func (b backend) Admin(ctx context.Context) (bool, error) { return b.s.Admin(ctx) }
func (b backend) Confirm(ctx context.Context) error       { return localauth.Confirm(ctx) }
func (b backend) Request(ctx context.Context, reason string) (int, error) {
	device, build, err := b.s.Device(ctx)
	if err != nil {
		return 0, err
	}
	client, profile, err := b.s.PrepareRequest(ctx, device, build)
	if err != nil {
		return 0, err
	}
	// Re-check before submission if elevation activated while loading the profile.
	if active, err := b.s.Admin(ctx); err != nil {
		return 0, err
	} else if active {
		return profile.Minutes, aod.ErrAlreadyActive
	}
	_, err = client.Request(ctx, profile, reason)
	return profile.Minutes, err
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, version, func(ctx context.Context) (cli.Backend, error) {
		s, err := aod.LocalSystem(ctx)
		return backend{s}, err
	}))
}
