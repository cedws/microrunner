package microrunner

import (
	"context"
	"fmt"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

var _ sandboxBackend = (*msbBackend)(nil)
var _ sandboxBackend = (*stubBackend)(nil)

type sandboxConfig struct {
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
	Kill(ctx context.Context) error
}

type msbBackend struct{}

func (b *msbBackend) CreateSandbox(ctx context.Context, name string, config sandboxConfig) error {
	opts := []msb.SandboxOption{
		msb.WithCPUs(config.CPU),
		msb.WithMemory(config.MemoryMiB),
		msb.WithImage(config.Image),
		msb.WithHostname(name),
		msb.WithReplace(),
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

	select {
	case exitCode := <-waitCh:
		return fmt.Errorf("sandbox exited early with exit code %d", exitCode)
	case <-time.After(5 * time.Second):
		return nil
	}
}

func (b *msbBackend) GetSandbox(ctx context.Context, name string) (sandbox, error) {
	return msb.GetSandbox(ctx, name)
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
		return nil, fmt.Errorf("sandbox %q not found", name)
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

func (s *stubSandbox) Kill(ctx context.Context) error {
	return nil
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
	// name := fmt.Sprintf("%s-%s", config.Prefix, uuid.New())
	name := uuid.New().String()

	if err := s.backend.CreateSandbox(ctx, name, config); err != nil {
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

	if sandbox.Kill(ctx) != nil {
		return err
	}

	if err := sandbox.Remove(ctx); err != nil {
		return err
	}

	return nil
}
