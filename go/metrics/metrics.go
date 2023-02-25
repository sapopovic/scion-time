package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	syncGlobalCorrN = "timeservice_sync_global_corr"
	syncGlobalCorrH = "The current clock correction applied based on global sync"
	syncLocalCorrN  = "timeservice_sync_local_corr"
	syncLocalCorrH  = "The current clock correction applied based on local sync"
)

func newGauge(name, help string) prometheus.Gauge {
	return promauto.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	})
}

func NewGlobalSyncCorrGauge() prometheus.Gauge {
	return newGauge(syncGlobalCorrN, syncGlobalCorrH)
}

func NewLocalSyncCorrGauge() prometheus.Gauge {
	return newGauge(syncLocalCorrN, syncLocalCorrH)
}