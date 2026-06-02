package cmd

import (
	"context"

	"go.cedwards.xyz/microrunner/pkg/microrunner"
)

type pruneCmd struct{}

func (l *pruneCmd) Run() error {
	return microrunner.Prune(context.Background())
}
