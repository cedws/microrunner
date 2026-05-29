package cmd

import (
	"fmt"

	"go.cedwards.xyz/microrunner/pkg/version"
)

type versionCmd struct{}

func (v *versionCmd) Run() error {
	fmt.Printf("microrunner %s\n", version.Version())
	return nil
}
