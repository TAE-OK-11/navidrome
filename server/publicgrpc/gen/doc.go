// Package gen contains protobuf and gRPC stubs for the public client API.
package gen

//go:generate sh -c "protoc -I ../../.. --go_out=. --go_opt=module=github.com/navidrome/navidrome/server/publicgrpc/gen --go-grpc_out=. --go-grpc_opt=module=github.com/navidrome/navidrome/server/publicgrpc/gen ../../../proto/navidrome/public/v1/public.proto"
