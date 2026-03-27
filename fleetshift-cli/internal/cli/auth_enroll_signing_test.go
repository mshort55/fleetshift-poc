package cli_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/fleetshift/fleetshift-poc/fleetshift-server/gen/fleetshift/v1"
)

type fakeSigningKeyBindingServer struct {
	pb.UnimplementedSigningKeyBindingServiceServer

	mu       sync.Mutex
	bindings map[string]*pb.SigningKeyBinding
}

func newFakeSigningKeyBindingServer() *fakeSigningKeyBindingServer {
	return &fakeSigningKeyBindingServer{
		bindings: make(map[string]*pb.SigningKeyBinding),
	}
}

func (s *fakeSigningKeyBindingServer) CreateSigningKeyBinding(_ context.Context, req *pb.CreateSigningKeyBindingRequest) (*pb.SigningKeyBinding, error) {
	if req.GetSigningKeyBindingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "signing_key_binding_id is required")
	}
	if len(req.GetKeyBindingDoc()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key_binding_doc is required")
	}
	if len(req.GetKeyBindingSignature()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key_binding_signature is required")
	}
	if req.GetIdentityToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "identity_token is required")
	}

	name := "signingKeyBindings/" + req.GetSigningKeyBindingId()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.bindings[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "signing key binding %q already exists", name)
	}

	binding := &pb.SigningKeyBinding{
		Name:                name,
		Subject:             "test-user",
		Issuer:              "https://issuer.example.com",
		PublicKeyJwk:        []byte(`{"kty":"EC"}`),
		Algorithm:           "ES256",
		KeyBindingDoc:       req.GetKeyBindingDoc(),
		KeyBindingSignature: req.GetKeyBindingSignature(),
		IdentityToken:       req.GetIdentityToken(),
		CreateTime:          timestamppb.Now(),
		ExpireTime:          timestamppb.Now(),
	}

	s.bindings[name] = binding
	return binding, nil
}

func TestAuthEnrollSigning_NoConfig(t *testing.T) {
	srv := grpc.NewServer()
	pb.RegisterSigningKeyBindingServiceServer(srv, newFakeSigningKeyBindingServer())
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(lis)
	t.Cleanup(func() { srv.GracefulStop() })

	_, err = runCLIErr(t, "--server", lis.Addr().String(), "auth", "enroll-signing")
	if err == nil {
		t.Fatal("expected error when no auth config exists")
	}
	output, _ := runCLIErr(t, "--server", lis.Addr().String(), "auth", "enroll-signing")
	if !strings.Contains(output, "auth config") && err == nil {
		t.Errorf("expected auth config error, got output: %s", output)
	}
}
