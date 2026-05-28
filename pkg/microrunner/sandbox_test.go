package microrunner

import (
	"fmt"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func newStubManager(t *testing.T) *sandboxManager {
	t.Helper()

	exitCh := make(chan sandboxExit, 1)
	t.Cleanup(func() {
		close(exitCh)
	})

	return &sandboxManager{
		backend: &stubBackend{
			exitCh: exitCh,
		},
		sandboxes: syncMap[string, struct{}]{},
	}
}

func TestRunnerManager(t *testing.T) {
	t.Run("spawns a sandbox", func(t *testing.T) {
		manager := newStubManager(t)
		assert.Zero(t, manager.Count())

		name, err := manager.Spawn(t.Context(), sandboxConfig{
			Name:      "test-runner",
			CPU:       1,
			MemoryMiB: 1024,
			Image:     "alpine",
		})
		assert.NoError(t, err)
		assert.NotZero(t, name)
		assert.Equal(t, 1, manager.Count())

		t.Run("has metrics", func(t *testing.T) {
			metrics := manager.Metrics(t.Context())
			assert.Equal(t, 1, len(metrics))
			assert.NotZero(t, metrics[name].CPUPercent)
		})

		t.Run("then destroys a sandbox", func(t *testing.T) {
			err := manager.Destroy(t.Context(), name)
			assert.NoError(t, err)
			assert.Zero(t, manager.Count())
		})
	})

	t.Run("shutdown destroys all sandboxes", func(t *testing.T) {
		manager := newStubManager(t)

		for i := range 3 {
			_, err := manager.Spawn(t.Context(), sandboxConfig{
				Name:      fmt.Sprintf("test-runner-%d", i),
				CPU:       1,
				MemoryMiB: 512,
				Image:     "alpine",
			})
			assert.NoError(t, err)
		}
		assert.Equal(t, 3, manager.Count())

		err := manager.Shutdown(t.Context())
		assert.NoError(t, err)
		assert.Zero(t, manager.Count())
	})

	t.Run("shutdown with no sandboxes", func(t *testing.T) {
		manager := newStubManager(t)
		assert.Zero(t, manager.Count())

		err := manager.Shutdown(t.Context())
		assert.NoError(t, err)
		assert.Zero(t, manager.Count())
	})

	t.Run("destroy nonexistent sandbox is a noop", func(t *testing.T) {
		manager := newStubManager(t)
		err := manager.Destroy(t.Context(), "nonexistent")
		assert.NoError(t, err)
	})

	t.Run("supervises exited sandbox", func(t *testing.T) {
		manager := newStubManager(t)
		backend := manager.backend.(*stubBackend)

		name, err := manager.Spawn(t.Context(), sandboxConfig{
			Name:      "test-runner",
			CPU:       1,
			MemoryMiB: 1024,
			Image:     "alpine",
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, manager.Count())

		backend.exitCh <- sandboxExit{ExitCode: 1}

		deadline := time.After(time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for manager.Count() != 0 {
			select {
			case <-deadline:
				t.Fatalf("sandbox %s was not removed from the pool", name)
			case <-ticker.C:
			}
		}
	})
}
