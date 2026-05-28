package microrunner

import (
	"context"
	"log/slog"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
)

var _ listener.Scaler = (*scaler)(nil)

type scaler struct {
	ssClient       scalesetClient
	scaleSetID     int
	sandboxManager *sandboxManager
	vmconfig       vmconfig
	log            *slog.Logger
}

func newScaler(ssClient scalesetClient, scaleSetID int, vmconfig vmconfig) *scaler {
	s := &scaler{
		ssClient:       ssClient,
		scaleSetID:     scaleSetID,
		sandboxManager: newMSBManager(ssClient, scaleSetID),
		vmconfig:       vmconfig,
		log: slog.With(slog.Group("scaleset",
			"label", vmconfig.label.Name,
			"id", scaleSetID,
		)),
	}

	return s
}

func (s *scaler) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down", "runners", s.sandboxManager.Count())
	return s.sandboxManager.Shutdown(ctx)
}

func (s *scaler) HandleJobStarted(ctx context.Context, jobInfo *scaleset.JobStarted) error {
	s.log.Info("job started", "runner_name", jobInfo.RunnerName)
	return nil
}

func (s *scaler) HandleJobCompleted(ctx context.Context, jobInfo *scaleset.JobCompleted) error {
	s.log.Info("job completed", "runner_name", jobInfo.RunnerName)
	return nil
}

func (s *scaler) HandleDesiredRunnerCount(ctx context.Context, desiredCount int) (int, error) {
	count := s.sandboxManager.Count()

	s.log.Debug("received desired runner count event", "current_runners", count, "desired_runners", desiredCount)

	switch {
	case count == desiredCount:
		return desiredCount, nil
	case count < desiredCount:
		add := desiredCount - count

		s.log.Info("received scale up request", "current_runners", count, "desired_runners", desiredCount)

		for range add {
			name := uuid.New().String()

			jitRunnerSetting := &scaleset.RunnerScaleSetJitRunnerSetting{Name: name}
			jitRunnerConfig, err := s.ssClient.GenerateJitRunnerConfig(ctx, jitRunnerSetting, s.scaleSetID)
			if err != nil {
				return s.sandboxManager.Count(), err
			}

			_, err = s.sandboxManager.Spawn(ctx, sandboxConfig{
				Name:      name,
				CPU:       uint8(s.vmconfig.cpu),
				MemoryMiB: uint32(s.vmconfig.memory * 1024),
				Image:     s.vmconfig.image,
				Prefix:    s.vmconfig.label.Name,
				Labels:    []string{s.vmconfig.label.Name},
				Env: map[string]string{
					"ACTIONS_RUNNER_INPUT_JITCONFIG":     jitRunnerConfig.EncodedJITConfig,
					"ACTIONS_RUNNER_PRINT_LOG_TO_STDOUT": "true",
				},
			})
			if err != nil {
				return s.sandboxManager.Count(), err
			}

			s.log.Info("spawned sandbox", "runners", s.sandboxManager.Count(), "desired_runners", desiredCount)
		}

		return s.sandboxManager.Count(), nil
	default:
		// noop, official example says no need to shutdown extra runners
		return count, nil
	}
}
