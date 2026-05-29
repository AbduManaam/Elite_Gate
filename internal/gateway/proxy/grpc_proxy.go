package proxy

import (
	"context"
	"io"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// connCache holds one reusable connection per backend address.
var (
	connMu    sync.Mutex
	connCache = map[string]*grpc.ClientConn{}
)

func getConn(addr string) (*grpc.ClientConn, error) {
	connMu.Lock()
	defer connMu.Unlock()
	if cc, ok := connCache[addr]; ok {
		return cc, nil
	}
	cc, err := grpc.Dial(addr,
		grpc.WithCodec(proxyCodec{}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	connCache[addr] = cc
	return cc, nil
}

// TransparentHandler forwards unknown RPCs to backendAddr using gRPC transparent proxying.
func TransparentHandler(backendAddr string) grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		// FIX: use grpc.Method(ctx) — grpc.MethodFromServerStream is not public API.
		fullMethod, ok := grpc.Method(stream.Context())
		if !ok {
			return status.Error(codes.Internal, "method not found")
		}

		md, _ := metadata.FromIncomingContext(stream.Context())
		outCtx := metadata.NewOutgoingContext(stream.Context(), md)

		// FIX: reuse connection instead of dialing per RPC.
		conn, err := getConn(backendAddr)
		if err != nil {
			return status.Errorf(codes.Unavailable, "dial backend: %v", err)
		}

		// FIX: use a cancellable context so both goroutines are torn down together.
		outCtx, cancel := context.WithCancel(outCtx)
		defer cancel()

		clientStream, err := grpc.NewClientStream(outCtx, &grpc.StreamDesc{
			StreamName:    fullMethod,
			ServerStreams: true,
			ClientStreams: true,
		}, conn, fullMethod)
		if err != nil {
			return status.Errorf(codes.Internal, "client stream: %v", err)
		}

		errCh := make(chan error, 2)

		// client -> backend
		go func() {
			err := copyStream(clientStream, stream)
			// FIX: signal to backend that the client is done sending.
			clientStream.CloseSend()
			errCh <- err
		}()

		// backend -> client
		go func() { errCh <- copyStream(stream, clientStream) }()

		// FIX: always drain both goroutines; cancel context on first real error
		// so the other goroutine is also unblocked.
		var firstErr error
		for i := 0; i < 2; i++ {
			if err := <-errCh; err != nil && err != io.EOF {
				if firstErr == nil {
					firstErr = err
					cancel()
				}
			}
		}
		return firstErr
	}
}

func copyStream(dst grpc.Stream, src grpc.Stream) error {
	for {
		msg := &rawFrame{}
		if err := src.RecvMsg(msg); err != nil {
			return err
		}
		if err := dst.SendMsg(msg); err != nil {
			return err
		}
	}
}

type rawFrame struct{ data []byte }

type proxyCodec struct{}

func (proxyCodec) Marshal(v interface{}) ([]byte, error) {
	if f, ok := v.(*rawFrame); ok {
		return f.data, nil
	}
	return nil, status.Error(codes.Internal, "invalid frame")
}

func (proxyCodec) Unmarshal(data []byte, v interface{}) error {
	if f, ok := v.(*rawFrame); ok {
		f.data = append([]byte(nil), data...)
		return nil
	}
	return status.Error(codes.Internal, "invalid frame")
}

func (proxyCodec) Name() string { return "proxy" }

// ResolveGRPCBackend picks upstream host from route table (longest prefix on service name).
func ResolveGRPCBackend(fullMethod string, upstreamByPrefix map[string]string) (string, bool) {
	service := strings.TrimPrefix(fullMethod, "/")
	if i := strings.Index(service, "/"); i > 0 {
		service = service[:i]
	}
	var best string
	bestLen := -1
	for prefix, addr := range upstreamByPrefix {
		if strings.HasPrefix(service, prefix) && len(prefix) > bestLen {
			best, bestLen = addr, len(prefix)
		}
	}
	return best, best != ""
}
