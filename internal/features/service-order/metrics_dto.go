package serviceorder

// serviceMetricResponse is one entry of the average-execution-time metric
// response (specs/service-order-metrics/design.md §1.5/§1.9).
type serviceMetricResponse struct {
	ServiceID              string  `json:"serviceId"`
	ServiceCode            int64   `json:"serviceCode"`
	ServiceName            string  `json:"serviceName"`
	ExecutionCount         int     `json:"executionCount"`
	AverageDurationMinutes float64 `json:"averageDurationMinutes"`
}

// AverageExecutionTimeResponse is the
// GET /api/v1/service-orders/metrics/average-execution-time response body.
// Services is built with a non-nil empty slice so it marshals as `[]`, never
// `null`, when nothing qualifies (requirements.md BR6).
type AverageExecutionTimeResponse struct {
	Services []serviceMetricResponse `json:"services"`
}

func toAverageExecutionTimeResponse(metrics []*ServiceMetric) AverageExecutionTimeResponse {
	services := make([]serviceMetricResponse, 0, len(metrics))
	for _, metric := range metrics {
		services = append(services, serviceMetricResponse{
			ServiceID:              metric.ServiceID.String(),
			ServiceCode:            metric.ServiceCode,
			ServiceName:            metric.ServiceName,
			ExecutionCount:         metric.ExecutionCount,
			AverageDurationMinutes: metric.AverageDurationMinutes,
		})
	}
	return AverageExecutionTimeResponse{Services: services}
}
