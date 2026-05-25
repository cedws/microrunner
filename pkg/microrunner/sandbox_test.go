package microrunner

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func newStubManager() *sandboxManager {
	return &sandboxManager{
		backend:   &stubBackend{},
		sandboxes: syncMap[string, struct{}]{},
	}
}

func TestRunnerManager(t *testing.T) {
	t.Run("spawns a sandbox", func(t *testing.T) {
		manager := newStubManager()
		assert.Zero(t, manager.Count())

		name, err := manager.Spawn(t.Context(), sandboxConfig{
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
		manager := newStubManager()

		for range 3 {
			_, err := manager.Spawn(t.Context(), sandboxConfig{
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
		manager := newStubManager()
		assert.Zero(t, manager.Count())

		err := manager.Shutdown(t.Context())
		assert.NoError(t, err)
		assert.Zero(t, manager.Count())
	})

	t.Run("destroy nonexistent sandbox is a noop", func(t *testing.T) {
		manager := newStubManager()
		err := manager.Destroy(t.Context(), "nonexistent")
		assert.NoError(t, err)
	})
}
