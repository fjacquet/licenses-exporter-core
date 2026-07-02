// Package licenses_core is the vendor-neutral engine for the licenses_exporter
// family: the license_ metric schema and its constructors, an immutable snapshot
// store, the collection loop, the Prometheus + OTLP export paths, and the HTTP
// server with cancelable validated hot reload. Vendor exporters implement Source
// and call Main.
package licenses_core
