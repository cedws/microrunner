package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/alecthomas/kong"
	"go.cedwards.xyz/microrunner/pkg/microrunner"
)

type cli struct {
	Debug bool

	Start      startCmd      `cmd:"" default:"1" name:"start"`
	Version    versionCmd    `cmd:"" name:"version"`
	Prune      pruneCmd      `cmd:"" name:"prune"`
	JSONSchema jsonschemaCmd `cmd:"" name:"jsonschema" help:"Print config JSON schema"`
}

type startCmd struct {
	microrunner.Config `embed:""`
	ConfigFile         []byte `name:"config-file" type:"existingfile" help:"Path to config file." json:"-"`
}

func (s *startCmd) Run(cli *cli) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}

	config := cli.Start.Config
	if cli.Start.ConfigFile != nil {
		if err := json.Unmarshal(cli.Start.ConfigFile, &config); err != nil {
			return err
		}
	}

	config.GitHubToken = githubToken
	config.Debug = cli.Debug

	if err := microrunner.Start(ctx, config); !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func Execute() {
	var cli cli

	ctx := kong.Parse(&cli, kong.Name("microrunner"), kong.UsageOnError())
	ctx.FatalIfErrorf(ctx.Run())
}
