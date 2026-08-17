package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	UploadsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "atlasstore_uploads_total",
		Help: "The total number of successful file uploads",
	})
	
	DownloadsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "atlasstore_downloads_total",
		Help: "The total number of successful file downloads",
	})

	ActiveNodes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atlasstore_active_nodes",
		Help: "The current number of active storage nodes in the Hash Ring",
	})
)