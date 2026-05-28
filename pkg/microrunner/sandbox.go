package microrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actions/scaleset"
	msb "github.com/superradcompany/microsandbox/sdk/go"
	"go.cedwards.xyz/microrunner/pkg/metrics"
)

var defaultDomains = []string{
	"github.com",
	"api.github.com",
	"*.actions.githubusercontent.com",
	"codeload.github.com",
	"results-receiver.actions.githubusercontent.com",
	"*.blob.core.windows.net",
	"objects.githubusercontent.com",
	"objects-origin.githubusercontent.com",
	"github-releases.githubusercontent.com",
	"github-registry-files.githubusercontent.com",
	"*.pkg.github.com",
	"pkg-containers.githubusercontent.com",
	"ghcr.io",
	"github-cloud.githubusercontent.com",
	"github-cloud.s3.amazonaws.com",
	"dependabot-actions.githubapp.com",
	"release-assets.githubusercontent.com",
	"api.snapcraft.io",
}

func makePolicyRules() []msb.PolicyRule {
	var rules []msb.PolicyRule

	for _, d := range defaultDomains {
		rules = append(rules, msb.PolicyRule{
			Action:      msb.PolicyActionAllow,
			Direction:   msb.PolicyDirectionEgress,
			Destination: strings.TrimPrefix(d, "*"),
			Port:        "443",
		})
	}

	return rules
}

var _ sandboxBackend = (*msbBackend)(nil)
var _ sandboxBackend = (*stubBackend)(nil)

type sandboxConfig struct {
	Name      string
	CPU       uint8
	MemoryMiB uint32
	Image     string
	Env       map[string]string
	Prefix    string
	Labels    []string
}

type sandboxBackend interface {
	CreateSandbox(ctx context.Context, name string, config sandboxConfig) (<-chan sandboxExit, error)
	GetSandbox(ctx context.Context, name string) (sandbox, error)
}

type sandboxExit struct {
	err        error
	execOutput *msb.ExecOutput
}

type sandbox interface {
	Stop(ctx context.Context) error
	Remove(ctx context.Context) error
	Metrics(ctx context.Context) (*Metrics, error)
}

type Metrics struct {
	*msb.Metrics
}

type msbBackend struct{}

func (b *msbBackend) CreateSandbox(ctx context.Context, name string, config sandboxConfig) (<-chan sandboxExit, error) {
	opts := []msb.SandboxOption{
		msb.WithCPUs(config.CPU),
		msb.WithMemory(config.MemoryMiB),
		msb.WithImage(config.Image),
		msb.WithHostname(name),
		msb.WithReplace(),
		msb.WithNetwork(&msb.NetworkConfig{
			Rules:         makePolicyRules(),
			DefaultEgress: msb.PolicyActionDeny,
		}),
	}

	sandbox, err := msb.CreateSandbox(ctx, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	stream, err := sandbox.ShellStream(ctx, "./run.sh", msb.WithExecEnv(config.Env))
	if err != nil {
		return nil, err
	}

	exitCh := make(chan sandboxExit, 1)
	go func() {
		output, err := stream.Collect(context.Background())
		exitCh <- sandboxExit{
			err:        err,
			execOutput: output,
		}
		close(exitCh)
	}()

	return exitCh, nil
}

func (b *msbBackend) GetSandbox(ctx context.Context, name string) (sandbox, error) {
	sandbox, err := msb.GetSandbox(ctx, name)
	if err != nil {
		return nil, err
	}
	return &msbSandbox{SandboxHandle: sandbox}, nil
}

type msbSandbox struct {
	*msb.SandboxHandle
}

func (s *msbSandbox) Metrics(ctx context.Context) (*Metrics, error) {
	m, err := s.SandboxHandle.Metrics(ctx)
	if err != nil {
		return nil, err
	}
	return &Metrics{m}, nil
}

type stubBackend struct {
	sandboxes syncMap[string, sandboxConfig]
	exitCh    chan sandboxExit
}

func (b *stubBackend) CreateSandbox(ctx context.Context, name string, config sandboxConfig) (<-chan sandboxExit, error) {
	b.sandboxes.Store(name, config)
	return b.exitCh, nil
}

func (b *stubBackend) GetSandbox(ctx context.Context, name string) (sandbox, error) {
	config, ok := b.sandboxes.Load(name)
	if !ok {
		return nil, fmt.Errorf("sandbox %s not found", name)
	}
	return &stubSandbox{
		config: config,
		deleteFunc: func() {
			b.sandboxes.Delete(name)
		},
	}, nil
}

type stubSandbox struct {
	config     sandboxConfig
	deleteFunc func()
}

func (s *stubSandbox) Stop(ctx context.Context) error {
	return nil
}

func (s *stubSandbox) Remove(ctx context.Context) error {
	s.deleteFunc()
	return nil
}

func (s *stubSandbox) Metrics(ctx context.Context) (*Metrics, error) {
	return &Metrics{
		Metrics: &msb.Metrics{
			CPUPercent:       25.0 + float64(s.config.CPU)*2,
			MemoryBytes:      uint64(s.config.MemoryMiB) / 2 * 1024 * 1024,
			MemoryLimitBytes: uint64(s.config.MemoryMiB) * 1024 * 1024,
			DiskReadBytes:    4096,
			DiskWriteBytes:   2048,
			NetRxBytes:       8192,
			NetTxBytes:       16384,
			Uptime:           5 * time.Minute,
		},
	}, nil
}

func newMSBManager(scalesetClient scalesetClient, scaleSetID int) *sandboxManager {
	m := &sandboxManager{
		backend:        &msbBackend{},
		scalesetClient: scalesetClient,
		scaleSetID:     scaleSetID,
		doneCh:         make(chan struct{}),
		closeOnce:      &sync.Once{},
	}
	go m.startMetrics()

	return m
}

type scalesetClient interface {
	GenerateJitRunnerConfig(ctx context.Context, jitRunnerSetting *scaleset.RunnerScaleSetJitRunnerSetting, scaleSetID int) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
}

type sandboxManager struct {
	backend        sandboxBackend
	sandboxes      syncMap[string, struct{}]
	scalesetClient scalesetClient
	scaleSetID     int
	doneCh         chan struct{}
	closeOnce      *sync.Once
}

func (m *sandboxManager) Metrics(ctx context.Context) map[string]*Metrics {
	result := make(map[string]*Metrics, m.sandboxes.Len())

	for name := range m.sandboxes.Iter() {
		sandbox, err := m.backend.GetSandbox(ctx, name)
		if err != nil {
			continue
		}
		metrics, err := sandbox.Metrics(ctx)
		if err != nil {
			continue
		}
		result[name] = metrics
	}

	return result
}

func (m *sandboxManager) UpdateMetrics(ctx context.Context) {
	for name, sandboxMetrics := range m.Metrics(ctx) {
		metrics.SandboxCPUPercent.
			WithLabelValues(name).
			Set(sandboxMetrics.CPUPercent)
		metrics.SandboxMemoryBytes.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.MemoryBytes))
		metrics.SandboxMemoryLimitBytes.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.MemoryLimitBytes))
		metrics.SandboxDiskReadBytes.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.DiskReadBytes))
		metrics.SandboxDiskWriteBytes.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.DiskWriteBytes))
		metrics.SandboxNetRxBytes.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.NetRxBytes))
		metrics.SandboxNetTxBytes.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.NetTxBytes))
		metrics.SandboxUptimeMs.
			WithLabelValues(name).
			Set(float64(sandboxMetrics.Uptime.Milliseconds()))
		metrics.SandboxTimestampMs.
			WithLabelValues(name).
			Set(float64(time.Now().UnixMilli()))
	}
}

func (s *sandboxManager) Count() int {
	return s.sandboxes.Len()
}

func (s *sandboxManager) Shutdown(ctx context.Context) error {
	if s.closeOnce != nil {
		s.closeOnce.Do(func() {
			close(s.doneCh)
		})
	}

	for name := range s.sandboxes.Iter() {
		if err := s.Destroy(ctx, name); err != nil {
			return err
		}
	}

	return nil
}

func (s *sandboxManager) Spawn(ctx context.Context, config sandboxConfig) (string, error) {
	name := config.Name

	exitCh, err := s.backend.CreateSandbox(ctx, config.Name, config)
	if err != nil {
		return "", err
	}
	s.sandboxes.Store(name, struct{}{})

	if exitCh != nil {
		go s.supervise(name, exitCh)
	}

	return name, nil
}

func (s *sandboxManager) Destroy(ctx context.Context, name string) error {
	if _, ok := s.sandboxes.Load(name); !ok {
		return nil
	}

	sandbox, err := s.backend.GetSandbox(ctx, name)
	if err != nil {
		return err
	}

	if err := sandbox.Stop(ctx); err != nil {
		return err
	}
	if err := sandbox.Remove(ctx); err != nil {
		return err
	}

	s.sandboxes.Delete(name)

	time.AfterFunc(30*time.Second, func() {
		s.destroyMetrics(name)
	})

	return nil
}

func (m *sandboxManager) destroyMetrics(name string) {
	metrics.SandboxCPUPercent.DeleteLabelValues(name)
	metrics.SandboxMemoryBytes.DeleteLabelValues(name)
	metrics.SandboxMemoryLimitBytes.DeleteLabelValues(name)
	metrics.SandboxDiskReadBytes.DeleteLabelValues(name)
	metrics.SandboxDiskWriteBytes.DeleteLabelValues(name)
	metrics.SandboxNetRxBytes.DeleteLabelValues(name)
	metrics.SandboxNetTxBytes.DeleteLabelValues(name)
	metrics.SandboxUptimeMs.DeleteLabelValues(name)
	metrics.SandboxTimestampMs.DeleteLabelValues(name)
}

func (m *sandboxManager) startMetrics() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.UpdateMetrics(context.Background())
		case <-m.doneCh:
			return
		}
	}
}

func (s *sandboxManager) supervise(name string, exitCh <-chan sandboxExit) {
	exit, ok := <-exitCh
	if !ok {
		return
	}
	slog.Info("sandbox process exited", "runner_name", name, "error", exit.err, "exit_code", exit.execOutput.ExitCode())

	if err := flushExecOutput(logsDir(), name, exit.execOutput); err != nil {
		return
	}

	if _, ok := s.sandboxes.Load(name); !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Destroy(ctx, name); err != nil {
		slog.Warn("failed to destroy exited sandbox", "error", err)
	}
}

type execOutput interface {
	StdoutBytes() []byte
	StderrBytes() []byte
}

func flushExecOutput(dir string, name string, output execOutput) error {
	var (
		sandboxLogsDir = filepath.Join(dir, name)
		stdoutPath     = filepath.Join(sandboxLogsDir, "stdout.txt")
		stderrPath     = filepath.Join(sandboxLogsDir, "stderr.txt")
	)

	if err := os.MkdirAll(sandboxLogsDir, 0o755); err != nil {
		return err
	}

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return err
	}
	defer stderrFile.Close()

	var (
		stdout = output.StdoutBytes()
		stderr = output.StderrBytes()
	)

	if _, err := stdoutFile.Write(stdout); err != nil {
		return err
	}
	if _, err := stderrFile.Write(stderr); err != nil {
		return err
	}

	return nil
}
