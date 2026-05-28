package microrunner

import (
	"context"
	"log/slog"
	"testing"

	"github.com/actions/scaleset"
	"github.com/alecthomas/assert/v2"
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
		ssClient:       &stubScalesetClient{},
		scaleSetID:     1,
		sandboxManager: newStubManager(t),
		vmconfig:       vm,
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
}
