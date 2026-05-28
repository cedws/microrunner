package microrunner

import (
	"context"
	"log/slog"
	"testing"

	"github.com/actions/scaleset"
	"github.com/alecthomas/assert/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.cedwards.xyz/microrunner/pkg/metrics"
)

type stubScalesetClient struct{}

func (s *stubScalesetClient) GenerateJitRunnerConfig(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
	return &scaleset.RunnerScaleSetJitRunnerConfig{
		EncodedJITConfig: "123_456_789",
	}, nil
}

var vm = vmconfig{
	label:  scaleset.Label{Name: "test"},
	cpu:    2,
	memory: 1,
}

func newStubScaler(t *testing.T) *scaler {
	t.Helper()

	return &scaler{
		sset:           &scaleset.RunnerScaleSet{ID: 1},
		ssClient:       &stubScalesetClient{},
		ssID:           1,
		sandboxManager: newStubManager(t),
		metricsRecorder: &scalesetMetricsRecorder{
			name: vm.label.Name,
		},
		vmconfig: vm,
		log: slog.With(slog.Group("scaleset",
			"label", vm.label.Name,
			"id", 1,
		)),
	}
}

func TestScaler(t *testing.T) {
	t.Run("handles desired runner count increase", func(t *testing.T) {
		scaler := newStubScaler(t)
		scaler.HandleDesiredRunnerCount(t.Context(), 1)
		assert.Equal(t, 1, scaler.sandboxManager.Count())

		t.Run("handles desired runner count decrease", func(t *testing.T) {
			// expect count to remain same, we don't bring down extra runners
			scaler.HandleDesiredRunnerCount(t.Context(), 1)
			assert.Equal(t, 1, scaler.sandboxManager.Count())
		})
	})

	t.Run("handles job complete", func(t *testing.T) {
		scaler := newStubScaler(t)
		scaler.HandleDesiredRunnerCount(t.Context(), 1)
		assert.Equal(t, 1, scaler.sandboxManager.Count())

		var name string
		for n := range scaler.sandboxManager.sandboxes.Iter() {
			name = n
			break
		}
		assert.NotZero(t, name)

		err := scaler.HandleJobStarted(t.Context(), &scaleset.JobStarted{
			RunnerName: name,
		})
		assert.NoError(t, err)

		err = scaler.HandleJobCompleted(t.Context(), &scaleset.JobCompleted{
			RunnerName: name,
		})
		assert.NoError(t, err)
	})

	t.Run("handles shutdown", func(t *testing.T) {
		scaler := newStubScaler(t)
		scaler.HandleDesiredRunnerCount(t.Context(), 1)
		assert.Equal(t, 1, scaler.sandboxManager.Count())

		err := scaler.Shutdown(t.Context())
		assert.NoError(t, err)
		assert.Equal(t, 0, scaler.sandboxManager.Count())
	})

	t.Run("spawns with jit config", func(t *testing.T) {
		scaler := newStubScaler(t)
		scaler.HandleDesiredRunnerCount(t.Context(), 1)

		for name := range scaler.sandboxManager.sandboxes.Iter() {
			sandbox, err := scaler.sandboxManager.backend.GetSandbox(t.Context(), name)
			assert.NoError(t, err)
			assert.Equal(t, "123_456_789", sandbox.(*stubSandbox).config.Env["ACTIONS_RUNNER_INPUT_JITCONFIG"])
		}
	})

	t.Run("records scaleset statistics", func(t *testing.T) {
		scaler := newStubScaler(t)

		scaler.metricsRecorder.RecordStatistics(&scaleset.RunnerScaleSetStatistic{
			TotalAvailableJobs:     1,
			TotalAcquiredJobs:      2,
			TotalAssignedJobs:      3,
			TotalRunningJobs:       4,
			TotalRegisteredRunners: 5,
			TotalBusyRunners:       6,
			TotalIdleRunners:       7,
		})

		assert.Equal(t, float64(1), testutil.ToFloat64(metrics.ScaleSetTotalAvailableJobs.WithLabelValues(vm.label.Name)))
		assert.Equal(t, float64(2), testutil.ToFloat64(metrics.ScaleSetTotalAcquiredJobs.WithLabelValues(vm.label.Name)))
		assert.Equal(t, float64(3), testutil.ToFloat64(metrics.ScaleSetTotalAssignedJobs.WithLabelValues(vm.label.Name)))
		assert.Equal(t, float64(4), testutil.ToFloat64(metrics.ScaleSetTotalRunningJobs.WithLabelValues(vm.label.Name)))
		assert.Equal(t, float64(5), testutil.ToFloat64(metrics.ScaleSetTotalRegisteredRunners.WithLabelValues(vm.label.Name)))
		assert.Equal(t, float64(6), testutil.ToFloat64(metrics.ScaleSetTotalBusyRunners.WithLabelValues(vm.label.Name)))
		assert.Equal(t, float64(7), testutil.ToFloat64(metrics.ScaleSetTotalIdleRunners.WithLabelValues(vm.label.Name)))

		scaler.metricsRecorder.Delete()
	})
}
