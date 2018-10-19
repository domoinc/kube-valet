package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Registry struct {
	packLeftPercentFullByNag map[string]*prometheus.GaugeVec
}

func NewRegistry() *Registry {
	return &Registry{
		packLeftPercentFullByNag: make(map[string]*prometheus.GaugeVec),
	}
}

func (r *Registry) GetPackLeftPercentFull(name string) *prometheus.GaugeVec {
	if _, ok := r.packLeftPercentFullByNag[name]; !ok {
		r.packLeftPercentFullByNag[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "kubevalet_packleft_full_percent",
			ConstLabels: prometheus.Labels{"node_assignment_group": name},
		}, []string{
			"node_assignment",
			"node_name",
			"pack_left_state",
		})
		prometheus.MustRegister(r.packLeftPercentFullByNag[name])
	}
	return r.packLeftPercentFullByNag[name]
}
