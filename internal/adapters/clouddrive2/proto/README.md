# CloudDrive2 official protocol snapshot

`clouddrive.proto` is an unmodified snapshot of the official CloudDrive2 API,
protocol version 1.1.2. Source, retrieval date and SHA-256 are in source.json.
No upstream site configuration, credentials or user data are included.

Generated files use protoc 36.2, protoc-gen-go v1.36.12 and
protoc-gen-go-grpc v1.6.2; the Go import path is supplied as a generator option
so the upstream source need not be altered. Unknown fields remain compatible
with normal protobuf evolution; actual deployed RPC availability is checked at
runtime, not inferred from this generated version.

Regeneration:

```sh
protoc -I internal/adapters/clouddrive2/proto -I <protoc-standard-include> \
  --go_out=internal/adapters/clouddrive2/proto --go_opt=paths=source_relative \
  --go_opt=Mclouddrive.proto=media-tool/internal/adapters/clouddrive2/proto \
  --go-grpc_out=internal/adapters/clouddrive2/proto --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=Mclouddrive.proto=media-tool/internal/adapters/clouddrive2/proto \
  clouddrive.proto
```

The snapshot is a protocol reference, not a statement granting additional
rights to CloudDrive2 itself. Review upstream terms before distributing a
commercial integration. Retain provenance when updating the snapshot.
