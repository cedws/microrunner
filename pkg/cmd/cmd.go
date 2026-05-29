package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/alecthomas/kong"
	"go.cedwards.xyz/microrunner/pkg/microrunner"
	"go.cedwards.xyz/microrunner/pkg/version"
)

type versionCmd struct{}

func (v *versionCmd) Run() error {
	fmt.Printf("microrunner %s\n", version.Version())
	return nil
}

type cli struct {
	GitHubConfigURL string `name:"github-config-url" required:"true"`
	MetricsAddr     string `help:"Listen address to serve prometheus metrics on"`
	Prefix          string `default:"microrunner"`
	Image           string `required:"true"`
	CPUMatrix       []int  `default:"2,4"`
	MemoryMatrix    []int  `default:"1,2"`
	GitHubToken     string `kong:"-"`
	MaxRunners      int    `default:"10"`
	Debug           bool
	Start           startCmd   `cmd:"" default:"1" name:"start"`
	Version         versionCmd `cmd:"" name:"version"`
}

type startCmd struct{}

func (s *startCmd) Run(cli *cli) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := microrunner.Start(ctx, microrunner.Config{
		GitHubToken:     cli.GitHubToken,
		GitHubConfigURL: cli.GitHubConfigURL,
		MetricsAddr:     cli.MetricsAddr,
		Prefix:          cli.Prefix,
		Image:           cli.Image,
		LabelMatrix: microrunner.LabelMatrix{
			CPU:       cli.CPUMatrix,
			MemoryMiB: cli.MemoryMatrix,
		},
		MaxRunners: cli.MaxRunners,
		Debug:      cli.Debug,
	})
	if !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func Execute() {
	var cli cli

	ctx := kong.Parse(&cli, kong.Name("microrunner"), kong.UsageOnError())
	cli.GitHubToken = os.Getenv("GITHUB_TOKEN")
	if cli.GitHubToken == "" {
		ctx.FatalIfErrorf(fmt.Errorf("GITHUB_TOKEN is required"))
	}
	ctx.FatalIfErrorf(ctx.Run())
}
