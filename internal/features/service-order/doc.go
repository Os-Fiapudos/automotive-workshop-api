// Package serviceorder implements the Service Order Opening feature:
// creating a service order for an active customer's active vehicle, always
// starting in the RECEIVED status, with an initial list of requested
// services and a creation history event recorded transactionally.
//
// See specs/service-order-opening/ for the requirements and design this
// package implements.
package serviceorder
