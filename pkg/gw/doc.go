// Package gw implements a generic gateway interface to Span. The gRPC command stream is realtively
// simple to implement but requires a bit of wiring that will be common for all gateway implementations
// and this package handles that. Implement the CommandHandler interface for new gateways to support
// more gateway types
package gw
