package microrunner

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
	"go.cedwards.xyz/microrunner/pkg/metrics"
)

var _ listener.Scaler = (*scaler)(nil)

type scaler struct {
	ssClient       scalesetClient
	scaleSetID     int
	sandboxManager *sandboxManager
	vmconfig       vmconfig
	doneCh         chan struct{}
	closeOnce      *sync.Once
}

func newScaler(ssClient scalesetClient, scaleSetID int, vmconfig vmconfig) *scaler {
	s := &scaler{
		ssClient:       ssClient,
		scaleSetID:     scaleSetID,
		sandboxManager: newMSBManager(ssClient, scaleSetID),
		vmconfig:       vmconfig,
		doneCh:         make(chan struct{}),
		closeOnce:      &sync.Once{},
	}

	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				s.updateMetrics(context.Background())
			case <-s.doneCh:
				return
			}
		}
	}()

	return s
}

func (s *scaler) updateMetrics(ctx context.Context) {
	for name, m := range s.sandboxManager.Metrics(ctx) {
		metrics.SandboxCPUPercent.
			WithLabelValues(name).
			Set(m.CPUPercent)
		metrics.SandboxMemoryBytes.
			WithLabelValues(name).
			Set(float64(m.MemoryBytes))
		metrics.SandboxMemoryLimitBytes.
			WithLabelValues(name).
			Set(float64(m.MemoryLimitBytes))
		metrics.SandboxDiskReadBytes.
			WithLabelValues(name).
			Set(float64(m.DiskReadBytes))
		metrics.SandboxDiskWriteBytes.
			WithLabelValues(name).
			Set(float64(m.DiskWriteBytes))
		metrics.SandboxNetRxBytes.
			WithLabelValues(name).
			Set(float64(m.NetRxBytes))
		metrics.SandboxNetTxBytes.
			WithLabelValues(name).
			Set(float64(m.NetTxBytes))
		metrics.SandboxUptimeMs.
			WithLabelValues(name).
			Set(float64(m.Uptime.Milliseconds()))
		metrics.SandboxTimestampMs.
			WithLabelValues(name).
			Set(float64(time.Now().UnixMilli()))
	}
}

func (s *scaler) Shutdown(ctx context.Context) error {
	slog.Info("shutting down", "runners", s.sandboxManager.Count())
	s.closeOnce.Do(func() {
		close(s.doneCh)
	})
	return s.sandboxManager.Shutdown(ctx)
}

func (s *scaler) HandleJobStarted(ctx context.Context, jobInfo *scaleset.JobStarted) error {
	slog.Info("job started", "runner_name", jobInfo.RunnerName)
	return nil
}

func (s *scaler) HandleJobCompleted(ctx context.Context, jobInfo *scaleset.JobCompleted) error {
	slog.Info("job completed", "runner_name", jobInfo.RunnerName)
	return s.sandboxManager.Destroy(ctx, jobInfo.RunnerName)
}

func (s *scaler) HandleDesiredRunnerCount(ctx context.Context, desiredCount int) (int, error) {
	count := s.sandboxManager.Count()

	slog.Info("received desired runner count event", "runners", count, "desired_runners", desiredCount)

	switch {
	case count == desiredCount:
		return desiredCount, nil
	case count < desiredCount:
		add := desiredCount - count

		for range add {
			name := uuid.New().String()

			jitRunnerSetting := &scaleset.RunnerScaleSetJitRunnerSetting{Name: name}
			jitRunnerConfig, err := s.ssClient.GenerateJitRunnerConfig(ctx, jitRunnerSetting, s.scaleSetID)
			if err != nil {
				return s.sandboxManager.Count(), err
			}

			_, err = s.sandboxManager.Spawn(ctx, sandboxConfig{
				CPU:       uint8(s.vmconfig.cpu),
				MemoryMiB: uint32(s.vmconfig.memory * 1024),
				Image:     s.vmconfig.image,
				Prefix:    s.vmconfig.label.Name,
				Labels:    []string{s.vmconfig.label.Name},
				Env: map[string]string{
					"ACTIONS_RUNNER_INPUT_JITCONFIG": jitRunnerConfig.EncodedJITConfig,
				},
			})
			if err != nil {
				return s.sandboxManager.Count(), err
			}

			slog.Info("spawned sandbox", "runners", s.sandboxManager.Count(), "desired_runners", desiredCount)
		}

		return s.sandboxManager.Count(), nil
	default:
		// noop, official example says no need to shutdown extra runners
		return count, nil
	}
}
