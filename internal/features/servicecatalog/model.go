package servicecatalog

import "time"

// Service is a service offered by the workshop (docs/entities.md "Service").
type Service struct {
	ID            string
	Code          int64
	Name          string
	Description   string
	Price         float64
	EstimatedTime *int // optional, in minutes (BR4)
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
