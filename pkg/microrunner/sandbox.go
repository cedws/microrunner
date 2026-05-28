package microrunner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/actions/scaleset"
	msb "github.com/superradcompany/microsandbox/sdk/go"
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
	CreateSandbox(ctx context.Context, name string, config sandboxConfig) error
	GetSandbox(ctx context.Context, name string) (sandbox, error)
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

func (b *msbBackend) CreateSandbox(ctx context.Context, name string, config sandboxConfig) error {
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
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	stream, err := sandbox.ShellStream(ctx, "./run.sh", msb.WithExecEnv(config.Env))
	if err != nil {
		return err
	}

	waitCh := make(chan int)

	go func() {
		code, err := stream.Wait(ctx)
		if err != nil {
			waitCh <- -1
		} else {
			waitCh <- code
		}
	}()

	// wait 5 seconds to see if sandbox startup fails, if it doesn't it's probably fine
	select {
	case exitCode := <-waitCh:
		return fmt.Errorf("sandbox exited early with exit code %d", exitCode)
	case <-time.After(5 * time.Second):
		return nil
	}
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
}

func (b *stubBackend) CreateSandbox(ctx context.Context, name string, config sandboxConfig) error {
	b.sandboxes.Store(name, config)
	return nil
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
	return &sandboxManager{
		backend:        &msbBackend{},
		scalesetClient: scalesetClient,
		scaleSetID:     scaleSetID,
	}
}

type scalesetClient interface {
	GenerateJitRunnerConfig(ctx context.Context, jitRunnerSetting *scaleset.RunnerScaleSetJitRunnerSetting, scaleSetID int) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
}

type sandboxManager struct {
	backend        sandboxBackend
	sandboxes      syncMap[string, struct{}]
	scalesetClient scalesetClient
	scaleSetID     int
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

func (s *sandboxManager) Count() int {
	return s.sandboxes.Len()
}

func (s *sandboxManager) Shutdown(ctx context.Context) error {
	for name := range s.sandboxes.Iter() {
		if err := s.Destroy(ctx, name); err != nil {
			return err
		}
	}

	return nil
}

func (s *sandboxManager) Spawn(ctx context.Context, config sandboxConfig) (string, error) {
	name := config.Name

	if err := s.backend.CreateSandbox(ctx, config.Name, config); err != nil {
		return "", err
	}
	s.sandboxes.Store(name, struct{}{})

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

	defer s.sandboxes.Delete(name)

	if err := sandbox.Stop(ctx); err != nil {
		return err
	}
	if err := sandbox.Remove(ctx); err != nil {
		return err
	}

	return nil
}
