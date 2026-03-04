// Package testserver provides a fully wired in-process FleetShift gRPC
// server for integration testing. The server uses SQLite in-memory storage
// and the synchronous workflow engine, making tests fast and deterministic.
package testserver

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/fleetshift/fleetshift-poc/fleetshift-server/gen/fleetshift/v1"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/application"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/delivery"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/sqlite"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/syncworkflow"
	transportgrpc "github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/transport/grpc"
)

// stubVerifier returns a fixed test identity for any token.
type stubVerifier struct{}

func (stubVerifier) Verify(_ context.Context, _ domain.OIDCConfig, _ string) (domain.SubjectClaims, error) {
	return domain.SubjectClaims{
		ID:     "test-user",
		Issuer: "test-issuer",
	}, nil
}

// stubDiscovery returns fixed test metadata.
type stubDiscovery struct{}

func (stubDiscovery) FetchMetadata(_ context.Context, issuerURL string) (domain.OIDCMetadata, error) {
	return domain.OIDCMetadata{
		Issuer:                issuerURL,
		AuthorizationEndpoint: issuerURL + "/authorize",
		TokenEndpoint:         issuerURL + "/token",
		JWKSURI:               issuerURL + "/jwks",
	}, nil
}

// Start launches an in-process gRPC server and returns its address.
// The server is stopped automatically when the test finishes.
func Start(t *testing.T) string {
	t.Helper()

	db := sqlite.OpenTestDB(t)
	store := &sqlite.Store{DB: db}

	router := delivery.NewRoutingDeliveryService()
	recording := &sqlite.RecordingDeliveryService{Store: store}
	router.Register("test", recording)

	owf := &domain.OrchestrationWorkflow{
		Store:      store,
		Delivery:   router,
		Strategies: domain.DefaultStrategyFactory{},
	}
	cwf := &domain.CreateDeploymentWorkflow{
		Store: store,
	}

	engine := &syncworkflow.Engine{}
	runners, err := engine.Register(owf, cwf)
	if err != nil {
		t.Fatalf("register workflows: %v", err)
	}
	deploymentSvc := &application.DeploymentService{
		Store:    store,
		CreateWF: runners.CreateDeployment,
	}

	authMethodRepo := &sqlite.AuthMethodRepo{DB: db}
	authMethodSvc := &application.AuthMethodService{
		Methods:   authMethodRepo,
		Discovery: stubDiscovery{},
	}
	authnInterceptor := transportgrpc.NewAuthnInterceptor(authMethodSvc, stubVerifier{})

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(authnInterceptor.Unary()),
		grpc.ChainStreamInterceptor(authnInterceptor.Stream()),
	)
	pb.RegisterDeploymentServiceServer(srv, &transportgrpc.DeploymentServer{
		Deployments: deploymentSvc,
	})
	pb.RegisterAuthMethodServiceServer(srv, &transportgrpc.AuthMethodServer{
		AuthMethods: authMethodSvc,
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go srv.Serve(lis)
	t.Cleanup(func() { srv.GracefulStop() })

	return lis.Addr().String()
}
