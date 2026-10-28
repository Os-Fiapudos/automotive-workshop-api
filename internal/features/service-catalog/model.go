package servicecatalog

import (
	"time"

	"automotive-workshop-api/internal/shared/money"
)

// Service is a service offered by the workshop (docs/entities.md "Service").
type Service struct {
	ID            string
	Code          int64
	Name          string
	Description   string
	Price         money.Cents
	EstimatedTime *int // optional, in minutes (BR4)
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
