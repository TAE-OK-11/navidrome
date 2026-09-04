// Package gen contains protobuf and gRPC stubs for Navidrome's internal
// integration API. Generated from proto/navidrome/integration/v1/*.proto.
package gen

//go:generate sh -c "protoc -I ../../.. --go_out=. --go_opt=module=github.com/navidrome/navidrome/core/integration/gen --go-grpc_out=. --go-grpc_opt=module=github.com/navidrome/navidrome/core/integration/gen ../../../proto/navidrome/integration/v1/integration.proto ../../../proto/navidrome/integration/v1/events.proto"
