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
	tier           tier
}

func newScaler(ssClient scalesetClient, scaleSetID int, tier tier) *scaler {
	return &scaler{
		ssClient:       ssClient,
		scaleSetID:     scaleSetID,
		sandboxManager: newMSBManager(ssClient, scaleSetID),
		tier:           tier,
	}
}

func (s *scaler) Shutdown(ctx context.Context) error {
	slog.Info("shutting down", "runners", s.sandboxManager.Count())
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
				CPU:       uint8(s.tier.cpu),
				MemoryMiB: uint32(s.tier.memory * 1024),
				Image:     s.tier.image,
				Prefix:    s.tier.label.Name,
				Labels:    []string{s.tier.label.Name},
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
