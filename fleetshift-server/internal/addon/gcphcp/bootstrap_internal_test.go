package gcphcp

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReadCACert_RespectsContextDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	accepted := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- struct{}{}
		<-release
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = readCACert(ctx, "https://"+listener.Addr().String())
	if err == nil {
		t.Fatal("expected context deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("readCACert took too long to fail: %v", elapsed)
	}

	select {
	case <-accepted:
	default:
		t.Fatal("expected listener to accept a connection")
	}
}

func TestCreatePlatformRBAC_ReconcilesExistingSubjects(t *testing.T) {
	client := fake.NewSimpleClientset(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: platformSAName + "-cluster-admin"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "wrong-service-account",
			Namespace: platformSANamespace,
		}},
	})

	if err := createPlatformRBAC(context.Background(), client); err != nil {
		t.Fatalf("createPlatformRBAC() error = %v", err)
	}

	got, err := client.RbacV1().ClusterRoleBindings().Get(context.Background(), platformSAName+"-cluster-admin", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get ClusterRoleBinding: %v", err)
	}

	wantSubjects := []rbacv1.Subject{{
		Kind:      "ServiceAccount",
		Name:      platformSAName,
		Namespace: platformSANamespace,
	}}
	if !reflect.DeepEqual(got.Subjects, wantSubjects) {
		t.Fatalf("subjects = %#v, want %#v", got.Subjects, wantSubjects)
	}
}

func TestCreatePlatformRBAC_RecreatesExistingBindingWhenRoleRefDiffers(t *testing.T) {
	client := fake.NewSimpleClientset(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: platformSAName + "-cluster-admin"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      platformSAName,
			Namespace: platformSANamespace,
		}},
	})

	if err := createPlatformRBAC(context.Background(), client); err != nil {
		t.Fatalf("createPlatformRBAC() error = %v", err)
	}

	got, err := client.RbacV1().ClusterRoleBindings().Get(context.Background(), platformSAName+"-cluster-admin", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get ClusterRoleBinding: %v", err)
	}

	if got.RoleRef.Name != "cluster-admin" {
		t.Fatalf("roleRef.name = %q, want cluster-admin", got.RoleRef.Name)
	}
}
