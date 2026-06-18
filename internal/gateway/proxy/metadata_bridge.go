package proxy

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// forward gRPC metadata (headers + trailers) from the upstream
// backend stream back to the downstream client stream.

type metadataSender interface {
	SendHeader(metadata.MD) error
	SetTrailer(metadata.MD)
}

// grpcInternalHeaders are headers managed by the gRPC runtime itself.
// They must never be forwarded verbatim through a proxy.
var grpcInternalHeaders = map[string]struct{}{
	"grpc-status":             {},
	"grpc-message":            {},
	"grpc-status-details-bin": {},
	"grpc-encoding":           {},
	"grpc-accept-encoding":    {},
	"content-type":            {},
}

// stripTransportKeys removes pseudo-headers (":authority", ":path", etc.)
// and gRPC runtime-managed fields from a metadata copy.
func stripTransportKeys(md metadata.MD) {
	for k := range md {
		if len(k) > 0 && k[0] == ':' {
			delete(md, k)
			continue
		}
		if _, blocked := grpcInternalHeaders[k]; blocked {
			delete(md, k)
		}
	}
}

// Forwards response headers from the upstream stream to the client.
// Waits for initial metadata and filters pseudo-headers before sending.
func forwardHeaders(dst metadataSender, clientStream grpc.ClientStream) error {
	hdr, err := clientStream.Header()
	if err != nil {
		return fmt.Errorf("reading upstream headers: %w", err)
	}

	// Clone the data to avoid changing the original.
	out := hdr.Copy()
	stripTransportKeys(out)

	if len(out) == 0 {
		return nil
	}
	return dst.SendHeader(out)
}

// Forwards upstream trailers to the client after the stream completes.
// grpc-status and grpc-message are handled separately from the returned error.
func forwardTrailers(dst metadataSender, clientStream grpc.ClientStream) {
	out := clientStream.Trailer().Copy()
	stripTransportKeys(out)

	if len(out) == 0 {
		return
	}
	dst.SetTrailer(out)
}
