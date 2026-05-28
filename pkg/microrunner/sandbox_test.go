package microrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	msb "github.com/superradcompany/microsandbox/sdk/go"
	"go.cedwards.xyz/microrunner/pkg/metrics"
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
		t.Setenv("XDG_STATE_HOME", t.TempDir())

		name, err := manager.Spawn(t.Context(), sandboxConfig{
			Name:      "test-runner",
			CPU:       1,
			MemoryMiB: 1024,
			Image:     "alpine",
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, manager.Count())

		backend.exitCh <- sandboxExit{execOutput: &msb.ExecOutput{}}

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

type mockExecOutput struct{}

func (mockExecOutput) StdoutBytes() []byte {
	return []byte("stdout")
}

func (mockExecOutput) StderrBytes() []byte {
	return []byte("stderr")
}

func TestFlushExecOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	output := mockExecOutput{}

	err := flushExecOutput(dir, "test", output)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "test", "stdout.txt"))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "test", "stderr.txt"))
	assert.NoError(t, err)
}

func TestSandboxMetricsRecorder(t *testing.T) {
	t.Parallel()

	recorder := &sandboxMetricsRecorder{}
	sandboxMetrics := &Metrics{
		Metrics: &msb.Metrics{
			CPUPercent:       27,
			MemoryBytes:      1024,
			MemoryLimitBytes: 2048,
			DiskReadBytes:    4096,
			DiskWriteBytes:   8192,
			NetRxBytes:       16384,
			NetTxBytes:       32768,
			Uptime:           5 * time.Second,
		},
	}

	recorder.Record("test-recorder", sandboxMetrics)

	assert.Equal(t, float64(27), testutil.ToFloat64(metrics.SandboxCPUPercent.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(1024), testutil.ToFloat64(metrics.SandboxMemoryBytes.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(2048), testutil.ToFloat64(metrics.SandboxMemoryLimitBytes.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(4096), testutil.ToFloat64(metrics.SandboxDiskReadBytes.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(8192), testutil.ToFloat64(metrics.SandboxDiskWriteBytes.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(16384), testutil.ToFloat64(metrics.SandboxNetRxBytes.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(32768), testutil.ToFloat64(metrics.SandboxNetTxBytes.WithLabelValues("test-recorder")))
	assert.Equal(t, float64(5000), testutil.ToFloat64(metrics.SandboxUptimeMs.WithLabelValues("test-recorder")))
	assert.NotZero(t, testutil.ToFloat64(metrics.SandboxTimestampMs.WithLabelValues("test-recorder")))

	recorder.Delete("test-recorder")
}
