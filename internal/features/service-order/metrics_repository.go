package serviceorder

import "context"

// findAverageExecutionTimeByService implements the aggregate query behind
// GET /api/v1/service-orders/metrics/average-execution-time
// (specs/service-order-metrics/design.md §1.6). Only completed executions
// (ended_at IS NOT NULL) participate — the same condition satisfies both
// "only completed executions" and "in-progress executions excluded"
// (requirements.md BR2/BR3), since an audit_services row has no other state
// (specs/service-order-execution/design.md §1.3). A service with no
// qualifying execution is absent from the result (INNER JOIN), not returned
// with a zero count (requirements.md BR4/BR6).
func (repository *PostgresServiceOrderRepository) findAverageExecutionTimeByService(ctx context.Context, filter MetricsFilter) ([]*ServiceMetric, error) {
	rows, err := repository.pool.Query(ctx,
		`SELECT s.id, s.code, s.name,
		        COUNT(*) AS execution_count,
		        AVG(EXTRACT(EPOCH FROM (a.ended_at - a.started_at)) / 60.0) AS average_duration_minutes
		 FROM audit_services a
		 JOIN services s ON s.id = a.service_id
		 WHERE a.ended_at IS NOT NULL
		   AND ($1::uuid IS NULL OR a.service_id = $1)
		   AND ($2::timestamptz IS NULL OR a.started_at >= $2)
		   AND ($3::timestamptz IS NULL OR a.started_at <= $3)
		 GROUP BY s.id, s.code, s.name
		 ORDER BY s.name`,
		filter.ServiceID, filter.StartDate, filter.EndDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*ServiceMetric
	for rows.Next() {
		metric := &ServiceMetric{}
		if err := rows.Scan(&metric.ServiceID, &metric.ServiceCode, &metric.ServiceName,
			&metric.ExecutionCount, &metric.AverageDurationMinutes); err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, rows.Err()
}
