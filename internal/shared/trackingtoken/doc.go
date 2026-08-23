// Package trackingtoken generates and hashes opaque, high-entropy tokens used
// to grant access to a single resource without the administrative JWT (see
// specs/service-order-tracking/design.md §2). It has no business logic of its
// own — Generate/Hash are pure crypto helpers shared by any feature that
// needs this kind of token, the same role internal/shared/token plays for
// JWTs.
package trackingtoken
