// Package servicetracking implements customer-facing service order tracking
// (RF12): GET /api/v1/acompanhamento/{codigo}, letting a customer check their
// own service order's status and milestones using a high-entropy tracking
// token instead of the administrative JWT. See
// specs/service-order-tracking/ for the full requirements and design.
package servicetracking
