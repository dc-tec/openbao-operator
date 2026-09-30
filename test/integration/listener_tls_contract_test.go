//go:build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const listenerTLSIntegrationVersion = "2.7.0"

func TestCRD_OpenBaoCluster_ListenerTLSPolicy(t *testing.T) {
	tests := []struct {
		name      string
		listener  map[string]any
		wantError string
	}{
		{name: "omitted", listener: map[string]any{}},
		{name: "hybrid", listener: map[string]any{
			"tlsMinVersion": "tls13", "tlsMaxVersion": "tls13",
			"tlsKeyExchangePreferences": []any{"X25519MLKEM768", "SecP256r1MLKEM768", "SecP384r1MLKEM1024"},
		}},
		{name: "mixed groups", listener: map[string]any{
			"tlsKeyExchangePreferences": []any{"X25519MLKEM768", "X25519"},
		}},
		{name: "classical groups", listener: map[string]any{
			"tlsKeyExchangePreferences": []any{"CurveP256", "CurveP384", "CurveP521", "X25519"},
		}},
		{name: "invalid minimum", listener: map[string]any{"tlsMinVersion": "tls11"}, wantError: "tlsMinVersion"},
		{name: "invalid maximum", listener: map[string]any{"tlsMaxVersion": "tls14"}, wantError: "tlsMaxVersion"},
		{name: "empty minimum", listener: map[string]any{"tlsMinVersion": ""}, wantError: "tlsMinVersion"},
		{name: "empty groups", listener: map[string]any{
			"tlsKeyExchangePreferences": []any{},
		}, wantError: "tlsKeyExchangePreferences"},
		{name: "duplicate groups", listener: map[string]any{
			"tlsMinVersion": "tls13", "tlsKeyExchangePreferences": []any{"X25519MLKEM768", "X25519MLKEM768"},
		}, wantError: "Duplicate"},
		{name: "unknown group", listener: map[string]any{
			"tlsMinVersion": "tls13", "tlsKeyExchangePreferences": []any{"X25519Kyber768Draft00"},
		}, wantError: "tlsKeyExchangePreferences"},
		{name: "pure MLKEM unsupported", listener: map[string]any{
			"tlsMinVersion": "tls13", "tlsKeyExchangePreferences": []any{"MLKEM1024"},
		}, wantError: "tlsKeyExchangePreferences"},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalClusterObj(newTestNamespace(t), fmt.Sprintf("listener-tls-%d", index))
			cluster.Spec.Version = listenerTLSIntegrationVersion
			object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
			require.NoError(t, err)
			candidate := &unstructured.Unstructured{Object: object}
			candidate.SetGroupVersionKind(openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"))
			require.NoError(t, unstructured.SetNestedMap(candidate.Object, tt.listener, "spec", "configuration", "listener"))
			err = k8sClient.Create(ctx, candidate, client.DryRunAll)
			if tt.wantError != "" {
				requireInvalidRequest(t, err)
				require.ErrorContains(t, err, tt.wantError)
				require.NotContains(t, err.Error(), "no such key")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVAP_OpenBaoCluster_ListenerTLSPolicy(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)
	tests := []struct {
		name               string
		version            string
		disableClusterTLS  bool
		disableListenerTLS bool
		configure          func(*openbaov1alpha1.ListenerConfig)
		wantError          string
	}{
		{name: "stable", version: listenerTLSIntegrationVersion},
		{name: "leading v and metadata", version: "v2.7.0+build.1"},
		{name: "future minor prerelease", version: "2.8.0-beta1"},
		{name: "older", version: "2.6.3", wantError: "requires OpenBao >= 2.7.0"},
		{name: "2.7 prerelease", version: "2.7.0-beta1", wantError: "requires OpenBao >= 2.7.0"},
		{name: "cluster TLS disabled", version: listenerTLSIntegrationVersion, disableClusterTLS: true,
			wantError: "requires TLS enabled"},
		{name: "listener TLS disabled", version: listenerTLSIntegrationVersion, disableListenerTLS: true,
			wantError: "requires TLS enabled"},
		{name: "reversed bounds", version: listenerTLSIntegrationVersion,
			configure: func(policy *openbaov1alpha1.ListenerConfig) {
				policy.TLSMaxVersion = openbaov1alpha1.TLSVersion12
			}, wantError: "greater than or equal"},
		{name: "PQ with default TLS minimum", version: listenerTLSIntegrationVersion,
			configure: func(policy *openbaov1alpha1.ListenerConfig) {
				policy.TLSMinVersion = ""
			}, wantError: "requires tlsMinVersion=tls13"},
		{name: "PQ with TLS12 minimum", version: listenerTLSIntegrationVersion,
			configure: func(policy *openbaov1alpha1.ListenerConfig) {
				policy.TLSMinVersion = openbaov1alpha1.TLSVersion12
			}, wantError: "requires tlsMinVersion=tls13"},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalClusterObj(namespace, fmt.Sprintf("listener-policy-%d", index))
			cluster.Spec.Version = tt.version
			cluster.Spec.TLS.Enabled = !tt.disableClusterTLS
			cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{Listener: &openbaov1alpha1.ListenerConfig{
				TLSMinVersion:             openbaov1alpha1.TLSVersion13,
				TLSDisable:                &tt.disableListenerTLS,
				TLSKeyExchangePreferences: []openbaov1alpha1.TLSKeyExchangeGroup{openbaov1alpha1.TLSKeyExchangeX25519MLKEM768},
			}}
			if tt.configure != nil {
				tt.configure(cluster.Spec.Configuration.Listener)
			}
			err := k8sClient.Create(ctx, cluster, client.DryRunAll)
			if tt.wantError != "" {
				requireAdmissionDenied(t, err)
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVAP_OpenBaoCluster_ListenerTLSUpgrade(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)
	cluster := newMinimalClusterObj(namespace, "listener-upgrade")
	cluster.Spec.Version = "2.6.3"
	require.NoError(t, k8sClient.Create(ctx, cluster))
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) { status.CurrentVersion = "2.6.3" })
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, cluster))
	cluster.Spec.Version = listenerTLSIntegrationVersion
	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{Listener: &openbaov1alpha1.ListenerConfig{
		TLSMinVersion:             openbaov1alpha1.TLSVersion13,
		TLSKeyExchangePreferences: []openbaov1alpha1.TLSKeyExchangeGroup{openbaov1alpha1.TLSKeyExchangeX25519MLKEM768},
	}}
	err := k8sClient.Update(ctx, cluster, client.DryRunAll)
	requireAdmissionDenied(t, err)
	require.ErrorContains(t, err, "Complete the OpenBao >= 2.7.0 upgrade")
	cluster.Spec.Configuration = nil
	require.NoError(t, k8sClient.Update(ctx, cluster))
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.CurrentVersion = listenerTLSIntegrationVersion
	})
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, cluster))
	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{Listener: &openbaov1alpha1.ListenerConfig{
		TLSMinVersion:             openbaov1alpha1.TLSVersion13,
		TLSKeyExchangePreferences: []openbaov1alpha1.TLSKeyExchangeGroup{openbaov1alpha1.TLSKeyExchangeX25519MLKEM768},
	}}
	require.NoError(t, k8sClient.Update(ctx, cluster))
}
