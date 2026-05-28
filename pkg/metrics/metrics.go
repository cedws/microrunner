package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SandboxCPUPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_cpu_percent",
		Help: "Current CPU usage percentage of the sandbox.",
	}, []string{"name"})
	SandboxMemoryBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_memory_bytes",
		Help: "Current memory usage of the sandbox in bytes.",
	}, []string{"name"})
	SandboxMemoryLimitBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_memory_limit_bytes",
		Help: "Memory limit of the sandbox in bytes.",
	}, []string{"name"})
	SandboxDiskReadBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_disk_read_bytes_total",
		Help: "Total bytes read from disk by the sandbox.",
	}, []string{"name"})
	SandboxDiskWriteBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_disk_write_bytes_total",
		Help: "Total bytes written to disk by the sandbox.",
	}, []string{"name"})
	SandboxNetRxBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_net_rx_bytes_total",
		Help: "Total bytes received over the network by the sandbox.",
	}, []string{"name"})
	SandboxNetTxBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_net_tx_bytes_total",
		Help: "Total bytes transmitted over the network by the sandbox.",
	}, []string{"name"})
	SandboxUptimeMs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_uptime_ms",
		Help: "Uptime of the sandbox in milliseconds.",
	}, []string{"name"})
	SandboxTimestampMs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sandbox_timestamp_ms",
		Help: "Timestamp of the last metrics observation in milliseconds.",
	}, []string{"name"})
	ScaleSetTotalAvailableJobs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_available_jobs",
		Help: "Current total available jobs for the scale set.",
	}, []string{"name"})
	ScaleSetTotalAcquiredJobs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_acquired_jobs",
		Help: "Current total acquired jobs for the scale set.",
	}, []string{"name"})
	ScaleSetTotalAssignedJobs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_assigned_jobs",
		Help: "Current total assigned jobs for the scale set.",
	}, []string{"name"})
	ScaleSetTotalRunningJobs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_running_jobs",
		Help: "Current total running jobs for the scale set.",
	}, []string{"name"})
	ScaleSetTotalRegisteredRunners = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_registered_runners",
		Help: "Current total registered runners for the scale set.",
	}, []string{"name"})
	ScaleSetTotalBusyRunners = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_busy_runners",
		Help: "Current total busy runners for the scale set.",
	}, []string{"name"})
	ScaleSetTotalIdleRunners = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "scaleset_total_idle_runners",
		Help: "Current total idle runners for the scale set.",
	}, []string{"name"})
)
