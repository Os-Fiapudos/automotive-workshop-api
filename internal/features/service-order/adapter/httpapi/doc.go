// Package httpapi is the service-order feature's HTTP adapter: route
// registration (RegisterRoutes), one handler file per group of use cases,
// the request/response shapes (the *_dto.go files), structural request
// validation, and the single mapping from domain/use-case errors to HTTP
// status codes and the apierror envelope (errors.go).
//
// HTTP concerns stop here: handlers parse and validate input, call the
// usecase package, and serialize its result. Dependencies point inward —
// this package imports usecase and domain, never the reverse.
package httpapi
