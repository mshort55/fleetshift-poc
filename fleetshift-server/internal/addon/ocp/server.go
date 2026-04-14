package ocp

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"

	ocpv1 "github.com/fleetshift/fleetshift-poc/gen/ocp/v1"
)

// Start launches the addon's internal callback gRPC server on the
// given address. The server uses its own token-auth interceptor —
// it does not share auth with the main fleetshift-server.
//
// Use "host:0" to bind to a random available port (useful for tests).
// After Start returns, CallbackAddr() returns the resolved address.
func (a *Agent) Start(callbackAddr string) error {
	lis, err := net.Listen("tcp", callbackAddr)
	if err != nil {
		return fmt.Errorf("ocp callback: listen %s: %w", callbackAddr, err)
	}

	a.callbackAddr = lis.Addr().String()

	srv := grpc.NewServer()
	ocpv1.RegisterCallbackServiceServer(srv, &callbackServer{
		provisions:    &a.provisions,
		tokenVerifier: a.tokenSigner,
	})

	a.grpcServer = srv

	go func() {
		_ = srv.Serve(lis)
	}()

	return nil
}

// Shutdown gracefully stops the callback gRPC server.
func (a *Agent) Shutdown(ctx context.Context) {
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}
}

// CallbackAddr returns the resolved address the callback server is
// listening on. Only valid after Start() returns.
func (a *Agent) CallbackAddr() string {
	return a.callbackAddr
}
