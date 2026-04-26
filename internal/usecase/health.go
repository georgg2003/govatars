package usecase

import (
	"context"
)

// healthProbe is checked by [Health.Status]. Implementations live in internal/repository/*.
type healthProbe interface {
	Health(ctx context.Context) error
}

// Health aggregates dependency checks for GET /health.
type Health struct {
	pg healthProbe
	s3 healthProbe
	mq healthProbe
}

// NewHealth builds a health collector. Pass nil to skip a component (reported as "skipped").
func NewHealth(pg, s3, mq healthProbe) *Health {
	return &Health{pg: pg, s3: s3, mq: mq}
}

// Status returns a JSON-serializable map for the health endpoint.
func (h *Health) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"status": "ok",
	}
	overallOK := true

	check := func(key string, p healthProbe) {
		if p == nil {
			out[key] = map[string]string{"status": "skipped"}
			return
		}
		if err := p.Health(ctx); err != nil {
			overallOK = false
			out[key] = map[string]string{"status": "down", "error": err.Error()}
			return
		}
		out[key] = map[string]string{"status": "ok"}
	}

	check("postgres", h.pg)
	check("s3", h.s3)
	check("rabbitmq", h.mq)

	if !overallOK {
		out["status"] = "degraded"
	}

	return out
}
