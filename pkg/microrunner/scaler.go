package microrunner

import (
	"context"
	"log/slog"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
	"go.cedwards.xyz/microrunner/pkg/metrics"
)

type scalesetMetricsRecorder struct {
	name string
}

func (r *scalesetMetricsRecorder) RecordStatistics(statistics *scaleset.RunnerScaleSetStatistic) {
	metrics.ScaleSetTotalAvailableJobs.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalAvailableJobs))
	metrics.ScaleSetTotalAcquiredJobs.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalAcquiredJobs))
	metrics.ScaleSetTotalAssignedJobs.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalAssignedJobs))
	metrics.ScaleSetTotalRunningJobs.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalRunningJobs))
	metrics.ScaleSetTotalRegisteredRunners.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalRegisteredRunners))
	metrics.ScaleSetTotalBusyRunners.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalBusyRunners))
	metrics.ScaleSetTotalIdleRunners.
		WithLabelValues(r.name).
		Set(float64(statistics.TotalIdleRunners))
}

func (r *scalesetMetricsRecorder) RecordJobStarted(*scaleset.JobStarted) {}

func (r *scalesetMetricsRecorder) RecordJobCompleted(*scaleset.JobCompleted) {}

func (r *scalesetMetricsRecorder) RecordDesiredRunners(int) {}

func (r *scalesetMetricsRecorder) Delete() {
	metrics.ScaleSetTotalAvailableJobs.DeleteLabelValues(r.name)
	metrics.ScaleSetTotalAcquiredJobs.DeleteLabelValues(r.name)
	metrics.ScaleSetTotalAssignedJobs.DeleteLabelValues(r.name)
	metrics.ScaleSetTotalRunningJobs.DeleteLabelValues(r.name)
	metrics.ScaleSetTotalRegisteredRunners.DeleteLabelValues(r.name)
	metrics.ScaleSetTotalBusyRunners.DeleteLabelValues(r.name)
	metrics.ScaleSetTotalIdleRunners.DeleteLabelValues(r.name)
}

var _ listener.Scaler = (*scaler)(nil)

type scaler struct {
	sset            *scaleset.RunnerScaleSet
	ssClient        scalesetClient
	ssID            int
	sandboxManager  *sandboxManager
	metricsRecorder *scalesetMetricsRecorder
	vmconfig        vmconfig
	log             *slog.Logger
}

func newScaler(ssClient scalesetClient, sset *scaleset.RunnerScaleSet, vmconfig vmconfig) *scaler {
	s := &scaler{
		sset:           sset,
		ssClient:       ssClient,
		ssID:           sset.ID,
		sandboxManager: newMSBManager(ssClient, sset.ID),
		metricsRecorder: &scalesetMetricsRecorder{
			name: vmconfig.label.Name,
		},
		vmconfig: vmconfig,
		log: slog.With(slog.Group("scaleset",
			"label", vmconfig.label.Name,
			"id", sset.ID,
		)),
	}

	return s
}

func (s *scaler) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down", "runners", s.sandboxManager.Count())
	s.metricsRecorder.Delete()
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
			jitRunnerConfig, err := s.ssClient.GenerateJitRunnerConfig(ctx, jitRunnerSetting, s.ssID)
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
