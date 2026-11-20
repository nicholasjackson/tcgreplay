// Package spaces is a minimal client for DigitalOcean Spaces over its
// S3-compatible API. It reads and writes whole objects by key, addressing
// them path-style (<endpoint>/<bucket>/<key>) and signing each request with
// AWS Signature Version 4, on the standard library alone.
//
// It knows nothing about cards. Card image copies, the admin's image
// clear, publishing card sets and the legacy clean-up build on it.
// Timeouts and failure policy belong to the caller: each call is bounded by
// the context it is given.
package spaces
