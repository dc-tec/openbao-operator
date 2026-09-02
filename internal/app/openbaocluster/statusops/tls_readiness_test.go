package statusops

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEvaluateTLSReadiness(t *testing.T) {
	t.Parallel()

	scheme := newTLSReadinessTestScheme(t)
	validCASecret, validServerSecret := newTLSReadinessTestSecrets(t)
	caKey := types.NamespacedName{Namespace: "default", Name: "example" + constants.SuffixTLSCA}
	serverKey := types.NamespacedName{Namespace: "default", Name: "example" + constants.SuffixTLSServer}
	readFailure := errors.New("injected read failure")

	tests := []struct {
		name       string
		configure  func(*openbaov1alpha1.OpenBaoCluster)
		objects    []client.Object
		readErrors map[types.NamespacedName]error
		want       ConditionResult
		wantReads  []types.NamespacedName
	}{
		{
			name: "tls disabled",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Enabled = false
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  ReasonDisabled,
				Message: "TLS is disabled",
			},
		},
		{
			name: "acme mode",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
			},
			want: ConditionResult{
				Status:  metav1.ConditionUnknown,
				Reason:  reasonUnknown,
				Message: "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness",
			},
		},
		{
			name: "missing ca secret",
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretMissing,
				Message: "CA TLS Secret is not present yet",
			},
			wantReads: []types.NamespacedName{caKey},
		},
		{
			name: "invalid ca secret",
			objects: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: caKey.Name, Namespace: caKey.Namespace},
				Data:       map[string][]byte{"ca.crt": []byte("not a certificate")},
			}},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretInvalid,
				Message: "CA TLS Secret is invalid: ca.crt is not valid PEM certificate data",
			},
			wantReads: []types.NamespacedName{caKey},
		},
		{
			name:       "ca secret read error",
			readErrors: map[types.NamespacedName]error{caKey: readFailure},
			want: ConditionResult{
				Status:  metav1.ConditionUnknown,
				Reason:  reasonUnknown,
				Message: "Failed to get CA TLS secret",
			},
			wantReads: []types.NamespacedName{caKey},
		},
		{
			name:    "missing server secret",
			objects: []client.Object{validCASecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretMissing,
				Message: "Server TLS Secret is not present yet",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name: "invalid server secret",
			objects: []client.Object{
				validCASecret.DeepCopy(),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: serverKey.Name, Namespace: serverKey.Namespace},
					Data:       map[string][]byte{"tls.crt": []byte("not a certificate")},
				},
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretInvalid,
				Message: "Server TLS Secret is invalid: tls.key is missing or empty",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name:       "server secret read error",
			objects:    []client.Object{validCASecret.DeepCopy()},
			readErrors: map[types.NamespacedName]error{serverKey: readFailure},
			want: ConditionResult{
				Status:  metav1.ConditionUnknown,
				Reason:  reasonUnknown,
				Message: "Failed to get TLS secret",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name: "valid operator-managed assets",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeOperatorManaged
			},
			objects: []client.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonReady,
				Message: "TLS assets are provisioned",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name:    "empty mode defaults to operator-managed",
			objects: []client.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonReady,
				Message: "TLS assets are provisioned",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name: "unrecognized mode follows operator-managed validation",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSMode("FutureMode")
			},
			objects: []client.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonReady,
				Message: "TLS assets are provisioned",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name: "operator-managed assets do not require external sans",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeOperatorManaged
				cluster.Spec.TLS.ExtraSANs = []string{"extra.example"}
			},
			objects: []client.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonReady,
				Message: "TLS assets are provisioned",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name: "valid external assets",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
			},
			objects: []client.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonReady,
				Message: "TLS assets are provisioned and valid",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
		{
			name: "invalid external assets",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
				cluster.Spec.TLS.ExtraSANs = []string{"extra.example"}
			},
			objects: []client.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretInvalid,
				Message: "External TLS assets are invalid: server certificate is missing required DNS SAN \"extra.example\"",
			},
			wantReads: []types.NamespacedName{caKey, serverKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := newOpenBaoClusterStatusTestObject()
			if tt.configure != nil {
				tt.configure(cluster)
			}
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
			cluster.Status.ObservedGeneration = 1
			clusterBefore := cluster.DeepCopy()

			reader := &tlsReadRecorder{
				Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build(),
				errors: tt.readErrors,
			}
			got := EvaluateTLSReadiness(t.Context(), reader, cluster)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EvaluateTLSReadiness() = %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(reader.reads, tt.wantReads) {
				t.Fatalf("Secret reads = %#v, want %#v", reader.reads, tt.wantReads)
			}
			if !reflect.DeepEqual(cluster, clusterBefore) {
				t.Fatalf("EvaluateTLSReadiness() mutated cluster: got %#v, want %#v", cluster, clusterBefore)
			}
		})
	}
}

type tlsReadRecorder struct {
	client.Reader
	errors map[types.NamespacedName]error
	reads  []types.NamespacedName
}

func (r *tlsReadRecorder) Get(
	ctx context.Context,
	key types.NamespacedName,
	obj client.Object,
	opts ...client.GetOption,
) error {
	r.reads = append(r.reads, key)
	if err := r.errors[key]; err != nil {
		return err
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func newTLSReadinessTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(client-go) error = %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(openbao) error = %v", err)
	}
	return scheme
}

func newTLSReadinessTestSecrets(t *testing.T) (*corev1.Secret, *corev1.Secret) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(ca) error = %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(server) error = %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-server"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"openbao-cluster-example.local"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(server) error = %v", err)
	}
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey() error = %v", err)
	}

	return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example" + constants.SuffixTLSCA,
				Namespace: "default",
			},
			Data: map[string][]byte{"ca.crt": caPEM},
		}, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example" + constants.SuffixTLSServer,
				Namespace: "default",
			},
			Data: map[string][]byte{
				"tls.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
				"tls.key": pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER}),
				"ca.crt":  caPEM,
			},
		}
}
