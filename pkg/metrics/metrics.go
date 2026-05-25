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
)
