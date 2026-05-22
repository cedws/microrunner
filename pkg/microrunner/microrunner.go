package microrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	msb "github.com/superradcompany/microsandbox/sdk/go"
	"go.cedwards.xyz/microrunner/pkg/version"
	"golang.org/x/sync/errgroup"
)

type LabelMatrix struct {
	CPU       []int
	MemoryMiB []int
}

type Config struct {
	GitHubToken     string
	GitHubConfigURL string
	Prefix          string
	Image           string
	LabelMatrix     LabelMatrix
	MaxRunners      int
	Debug           bool
}

func (c Config) Validate() error {
	if c.Prefix == "" {
		return fmt.Errorf("prefix is required")
	}
	return nil
}

func Start(ctx context.Context, config Config) error {
	if err := msb.EnsureInstalled(ctx); err != nil {
		return err
	}

	if err := config.Validate(); err != nil {
		return err
	}

	ssClient, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{
		GitHubConfigURL:     config.GitHubConfigURL,
		PersonalAccessToken: config.GitHubToken,
		SystemInfo: scaleset.SystemInfo{
			System:    "listener",
			Subsystem: "microrunner",
			CommitSHA: version.Commit(),
			Version:   version.Version(),
		},
	})
	if err != nil {
		return err
	}

	vmconfigs := makeVMConfigs(config.LabelMatrix, config.Prefix, config.Image)

	errGroup, ctx := errgroup.WithContext(ctx)

	for _, vmconfig := range vmconfigs {
		errGroup.Go(func() error {
			return createScaleSet(ctx, ssClient, config, vmconfig)
		})
	}

	return errGroup.Wait()
}

func createScaleSet(ctx context.Context, ssClient *scaleset.Client, config Config, vmconfig vmconfig) error {
	sset, err := ssClient.GetRunnerScaleSet(ctx, 1, vmconfig.label.Name)
	if err != nil || sset == nil {
		sset, err = ssClient.CreateRunnerScaleSet(ctx, &scaleset.RunnerScaleSet{
			RunnerGroupID: 1,
			Name:          vmconfig.label.Name,
			Labels:        []scaleset.Label{vmconfig.label},
		})
		if err != nil {
			return err
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	sessionClient, err := ssClient.MessageSessionClient(ctx, sset.ID, hostname)
	if err != nil {
		return err
	}

	var listenerLogger *slog.Logger
	if config.Debug {
		listenerLogger = slog.Default()
	}

	listener, err := listener.New(sessionClient, listener.Config{
		ScaleSetID: sset.ID,
		MaxRunners: max(config.MaxRunners, 0),
		Logger:     listenerLogger,
	})
	if err != nil {
		return err
	}

	slog.Info("starting scaleset", "label", vmconfig.label.Name, "scale_set_id", sset.ID)

	defer func() {
		slog.Info("stopping scaleset", "label", vmconfig.label.Name, "scale_set_id", sset.ID)
		if err := ssClient.DeleteRunnerScaleSet(context.Background(), sset.ID); err != nil {
			slog.Error("failed to delete runner scale set", "scale_set_id", sset.ID, "error", err)
		}
	}()
	defer sessionClient.Close(context.Background())

	return listener.Run(ctx, newScaler(ssClient, sset.ID, vmconfig))
}

type vmconfig struct {
	label  scaleset.Label
	image  string
	cpu    int
	memory int
}

func makeVMConfigs(matrix LabelMatrix, prefix string, image string) []vmconfig {
	var vmconfigs []vmconfig

	for _, cpu := range matrix.CPU {
		for _, mem := range matrix.MemoryMiB {
			vmconfigs = append(vmconfigs, vmconfig{
				label:  scaleset.Label{Name: fmt.Sprintf("%s-%dc-%dg", prefix, cpu, mem)},
				image:  image,
				cpu:    cpu,
				memory: mem,
			})
		}
	}

	return vmconfigs
}
