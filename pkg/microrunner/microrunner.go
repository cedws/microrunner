package microrunner

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	msb "github.com/superradcompany/microsandbox/sdk/go"
	"go.cedwards.xyz/microrunner/pkg/version"
	"golang.org/x/sync/errgroup"
)

func logsDir() string {
	path := os.Getenv("XDG_STATE_HOME")
	if path != "" {
		return filepath.Join(path, "microrunner")
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		dir = os.TempDir()
	}

	return filepath.Join(dir, ".local", "state", "microrunner")
}

func Start(ctx context.Context, config Config) error {
	if err := msbPreflight(ctx); err != nil {
		return fmt.Errorf("msb preflight check failed: %w", err)
	}
	var (
		runtimeVersion, _ = msb.RuntimeVersion()
		sdkVersion        = msb.SDKVersion()
	)
	slog.Info("sandbox preflight check passed", "msb_runtime_version", runtimeVersion, "msb_sdk_version", sdkVersion)

	if err := config.Validate(); err != nil {
		return err
	}

	useDefault := config.Egress.UseDefaultRules == nil || *config.Egress.UseDefaultRules
	slog.Info("loaded egress rules", "use_default", useDefault, "rules", len(config.Egress.Rules))

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

	vmconfigs := makeVMConfigs(config.LabelMatrix, config.Prefix, config.Image, config.Egress)

	errGroup, ctx := errgroup.WithContext(ctx)

	for _, vmconfig := range vmconfigs {
		errGroup.Go(func() error {
			return createScaleSet(ctx, ssClient, config, vmconfig)
		})
	}

	if config.MetricsAddr != "" {
		errGroup.Go(func() error {
			return startMetricsServer(ctx, config.MetricsAddr)
		})
	}

	return errGroup.Wait()
}

func startMetricsServer(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			Registry: prometheus.DefaultRegisterer,
		},
	))

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func msbPreflight(ctx context.Context) error {
	if err := msb.EnsureInstalled(ctx); err != nil {
		return err
	}

	opts := []msb.SandboxOption{
		msb.WithCPUs(1),
		msb.WithImage("alpine"),
		msb.WithMaxDuration(10 * time.Second),
		msb.WithMemory(128),
		msb.WithReplace(),
	}
	sandbox, err := msb.CreateSandbox(ctx, "microrunner", opts...)
	if err != nil {
		return err
	}
	if _, err := sandbox.StopAndWait(ctx); err != nil {
		return err
	}
	if err := sandbox.Close(); err != nil {
		return err
	}

	return msb.RemoveSandbox(ctx, "microrunner")
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

	scaler := newScaler(ssClient, sset, vmconfig)

	listenerConfig := listener.Config{
		ScaleSetID: sset.ID,
		MaxRunners: max(config.MaxRunners, 0),
		Logger:     listenerLogger,
	}
	listener, err := listener.New(sessionClient, listenerConfig, listener.WithMetricsRecorder(scaler.metricsRecorder))
	if err != nil {
		return err
	}

	slog.Info("starting scaleset", "label", vmconfig.label.Name, "id", sset.ID)

	defer func() {
		slog.Info("stopping scaleset", "label", vmconfig.label.Name, "id", sset.ID)
		if err := ssClient.DeleteRunnerScaleSet(context.Background(), sset.ID); err != nil {
			slog.Error("failed to delete runner scale set", "id", sset.ID, "error", err)
		}
	}()
	defer sessionClient.Close(context.Background())
	defer scaler.Shutdown(context.Background())

	return listener.Run(ctx, scaler)
}

type vmconfig struct {
	label  scaleset.Label
	image  string
	cpu    int
	memory int
	egress EgressConfig
}

func makeVMConfigs(matrix LabelMatrix, prefix string, image string, egress EgressConfig) []vmconfig {
	var vmconfigs []vmconfig

	for _, cpu := range matrix.CPU {
		for _, mem := range matrix.Memory {
			vmconfigs = append(vmconfigs, vmconfig{
				label:  scaleset.Label{Name: fmt.Sprintf("%s-%dc-%dg", prefix, cpu, mem)},
				image:  image,
				cpu:    cpu,
				memory: mem,
				egress: egress,
			})
		}
	}

	return vmconfigs
}
