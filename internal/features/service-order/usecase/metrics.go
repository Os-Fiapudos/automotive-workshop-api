package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MetricsFilter carries the already-parsed/validated filters accepted by
// GET /api/v1/service-orders/metrics/average-execution-time
// (specs/service-order-metrics/design.md §1.3). A nil field means "no
// filter."
type MetricsFilter struct {
	ServiceID *uuid.UUID
	StartDate *time.Time
	EndDate   *time.Time
}

// ServiceMetric is one row of the average-execution-time metric: a service
// with at least one completed execution, its execution count, and its
// average duration in minutes (design.md §1.6/§1.8).
type ServiceMetric struct {
	ServiceID              uuid.UUID
	ServiceCode            int64
	ServiceName            string
	ExecutionCount         int
	AverageDurationMinutes float64
}

// Metrics is the service-order metrics use case (specs/service-order-metrics/).
type Metrics struct {
	reader ExecutionMetricsReader
}

func NewMetrics(reader ExecutionMetricsReader) *Metrics {
	return &Metrics{reader: reader}
}

// AverageExecutionTime reports, per service, how many completed executions
// exist and their average duration (requirements.md BR1-BR4) — a thin
// pass-through, since the business rules are all satisfied by the
// aggregate query itself (design.md §1.6).
func (metrics *Metrics) AverageExecutionTime(ctx context.Context, filter MetricsFilter) ([]*ServiceMetric, error) {
	return metrics.reader.FindAverageExecutionTimeByService(ctx, filter)
}
