package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Ehsanamn1/alpha/internal/config"
	"github.com/Ehsanamn1/alpha/internal/health"
	"github.com/Ehsanamn1/alpha/internal/model"
)

type Status struct {
	ID        string
	Name      string
	Address   string
	Priority  int
	Healthy   bool
	Latency   time.Duration
	Error     string
	LastCheck time.Time
	Failures  int
}

type Manager struct {
	cfg    config.Config
	mu     sync.RWMutex
	status map[string]Status
}

func New(cfg config.Config) *Manager {
	m := &Manager{cfg: cfg, status: make(map[string]Status)}
	for _, e := range cfg.Engines {
		if e.Enabled {
			m.status[e.ID] = Status{ID: e.ID, Name: e.Name, Address: e.SOCKSAddr, Priority: e.Priority, Healthy: false, Error: "not checked yet"}
		}
	}
	return m
}

func ProbeProfiles(ctx context.Context, cfg config.Config) []Status {
	var enabled []config.EngineConfig
	for _, e := range cfg.Engines {
		if e.Enabled {
			enabled = append(enabled, e)
		}
	}
	statuses := make([]Status, len(enabled))
	var wg sync.WaitGroup
	for i, spec := range enabled {
		wg.Add(1)
		go func(i int, spec config.EngineConfig) {
			defer wg.Done()
			latency, err := health.ProbeAny(ctx, spec.SOCKSAddr, cfg.ProbeTargets())
			st := Status{ID: spec.ID, Name: spec.Name, Address: spec.SOCKSAddr, Priority: spec.Priority, Healthy: err == nil, Latency: latency, LastCheck: time.Now()}
			if err != nil {
				st.Error = err.Error()
			}
			statuses[i] = st
		}(i, spec)
	}
	wg.Wait()
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Healthy != statuses[j].Healthy {
			return statuses[i].Healthy
		}
		if statuses[i].Priority != statuses[j].Priority {
			return statuses[i].Priority < statuses[j].Priority
		}
		return statuses[i].Latency < statuses[j].Latency
	})
	return statuses
}

// CheckNow runs one end-to-end health pass and returns when it completes.
// It is used by the local dashboard's explicit "recheck" action.
func (m *Manager) CheckNow(ctx context.Context) {
	m.probeAll(ctx)
}

func (m *Manager) Monitor(ctx context.Context) {
	m.probeAll(ctx)
	ticker := time.NewTicker(m.cfg.ProbeInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.probeAll(ctx)
		}
	}
}

func (m *Manager) probeAll(parent context.Context) {
	var engines []config.EngineConfig
	for _, e := range m.cfg.Engines {
		if e.Enabled {
			engines = append(engines, e)
		}
	}
	var wg sync.WaitGroup
	for _, spec := range engines {
		wg.Add(1)
		go func(spec config.EngineConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(parent, m.cfg.ProbeTimeout())
			defer cancel()
			latency, err := health.ProbeAny(ctx, spec.SOCKSAddr, m.cfg.ProbeTargets())
			if err != nil {
				m.MarkFailure(spec.ID, err)
			} else {
				m.MarkSuccess(spec.ID, latency)
			}
		}(spec)
	}
	wg.Wait()
}

func (m *Manager) MarkFailure(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return
	}
	st.Failures++
	st.Error = err.Error()
	st.LastCheck = time.Now()
	if st.Failures >= m.cfg.FailureThreshold {
		st.Healthy = false
	}
	m.status[id] = st
}

func (m *Manager) MarkSuccess(id string, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return
	}
	st.Healthy = true
	st.Failures = 0
	st.Error = ""
	st.Latency = latency
	st.LastCheck = time.Now()
	m.status[id] = st
}

func (m *Manager) HealthyBackends() []model.Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	out := make([]model.Backend, 0, len(m.status))
	for _, st := range m.status {
		if st.Healthy && now.Sub(st.LastCheck) <= 3*m.cfg.ProbeInterval() {
			out = append(out, model.Backend{ID: st.ID, Name: st.Name, Address: st.Address, Priority: st.Priority, Latency: st.Latency})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].Latency < out[j].Latency
	})
	return out
}

func (m *Manager) MarkBackendFailure(b model.Backend, err error) { m.MarkFailure(b.ID, err) }
func (m *Manager) MarkBackendSuccess(b model.Backend)            { m.MarkSuccess(b.ID, b.Latency) }

func (m *Manager) Snapshot() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.status))
	for _, st := range m.status {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Healthy != out[j].Healthy {
			return out[i].Healthy
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].Latency < out[j].Latency
	})
	return out
}

func (m *Manager) String() string {
	sts := m.Snapshot()
	if len(sts) == 0 {
		return "no enabled engines"
	}
	ok := 0
	for _, s := range sts {
		if s.Healthy {
			ok++
		}
	}
	return fmt.Sprintf("%d/%d routes healthy", ok, len(sts))
}
