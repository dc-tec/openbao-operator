package openbaocluster

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func newOpenBaoClusterTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add appsv1 scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add openbao scheme: %v", err)
	}
	return scheme
}

func newOpenBaoClusterStatusTestObject() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "example",
			Namespace:       "default",
			Generation:      2,
			ResourceVersion: "1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
			Profile:  openbaov1alpha1.ProfileHardened,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
			},
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
			},
		},
	}
}

func TestContainsFinalizerAndRemoveFinalizer(t *testing.T) {
	t.Parallel()

	finalizers := []string{"alpha", openbaov1alpha1.OpenBaoClusterFinalizer, "beta", openbaov1alpha1.OpenBaoClusterFinalizer}
	if !containsFinalizer(finalizers, openbaov1alpha1.OpenBaoClusterFinalizer) {
		t.Fatal("expected containsFinalizer to find the requested finalizer")
	}
	if containsFinalizer(finalizers, "missing") {
		t.Fatal("containsFinalizer unexpectedly matched missing value")
	}

	got := removeFinalizer(finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("removeFinalizer() = %v, want [alpha beta]", got)
	}
}

func TestShouldEmitSecurityWarning(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "no annotations", annotations: nil, want: true},
		{name: "missing key", annotations: map[string]string{"other": "value"}, want: true},
		{name: "blank value", annotations: map[string]string{annotationLastDevelopmentWarning: "   "}, want: true},
		{name: "invalid timestamp", annotations: map[string]string{annotationLastDevelopmentWarning: "not-a-time"}, want: true},
		{name: "old timestamp", annotations: map[string]string{annotationLastDevelopmentWarning: now.Add(-securityWarningInterval).Add(-time.Minute).Format(time.RFC3339Nano)}, want: true},
		{name: "recent timestamp", annotations: map[string]string{annotationLastDevelopmentWarning: now.Add(-time.Minute).Format(time.RFC3339Nano)}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldEmitSecurityWarning(tt.annotations, annotationLastDevelopmentWarning, now); got != tt.want {
				t.Fatalf("shouldEmitSecurityWarning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmitSecurityWarningEvents_RecordsAndPersistsTimestamps(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
	cluster.Spec.SelfInit = nil
	cluster.Annotations = map[string]string{}

	recorder := events.NewFakeRecorder(10)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	reconciler := &OpenBaoClusterReconciler{
		Client:   k8sClient,
		Recorder: recorder,
	}

	if err := reconciler.emitSecurityWarningEvents(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("emitSecurityWarningEvents() error = %v", err)
	}

	select {
	case event := <-recorder.Events:
		if event == "" {
			t.Fatal("expected a warning event to be recorded")
		}
	default:
		t.Fatal("expected at least one event")
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	for _, key := range []string{
		annotationLastDevelopmentWarning,
		annotationLastStaticUnsealWarning,
		annotationLastRootTokenWarning,
	} {
		if updated.Annotations[key] == "" {
			t.Fatalf("expected annotation %q to be persisted", key)
		}
	}
}

func TestUpdateStatusForPausedAndProfileNotSet(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	t.Run("paused cluster gets paused conditions", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: client}

		if err := reconciler.updateStatusForPaused(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForPaused() error = %v", err)
		}
		if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseInitializing {
			t.Fatalf("phase = %s, want Initializing", cluster.Status.Phase)
		}
		for _, conditionType := range []openbaov1alpha1.ConditionType{
			openbaov1alpha1.ConditionAvailable,
			openbaov1alpha1.ConditionDegraded,
			openbaov1alpha1.ConditionTLSReady,
		} {
			cond := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
			if cond == nil {
				t.Fatalf("expected condition %s", conditionType)
			}
		}
	})

	t.Run("missing profile marks cluster blocked", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = ""
		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: client}

		if err := reconciler.updateStatusForProfileNotSet(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForProfileNotSet() error = %v", err)
		}
		if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseInitializing {
			t.Fatalf("phase = %s, want Initializing", cluster.Status.Phase)
		}
		productionReady := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady))
		if productionReady == nil || productionReady.Reason != ReasonProfileNotSet {
			t.Fatalf("production-ready condition = %#v, want reason %q", productionReady, ReasonProfileNotSet)
		}
	})
}

func TestSetTLSReadyCondition(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		objects       []runtime.Object
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "tls disabled",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.TLS.Enabled = false
				return cluster
			}(),
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonDisabled,
			wantMessageIn: "disabled",
		},
		{
			name: "acme mode",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
				return cluster
			}(),
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    reasonUnknown,
			wantMessageIn: "ACME",
		},
		{
			name:          "missing secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonTLSSecretMissing,
			wantMessageIn: "not present yet",
		},
		{
			name:          "invalid secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			objects:       []runtime.Object{&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example" + constants.SuffixTLSServer, Namespace: "default"}, Data: map[string][]byte{"tls.crt": []byte("cert")}}},
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonTLSSecretInvalid,
			wantMessageIn: "missing required keys",
		},
		{
			name:          "valid secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			objects:       []runtime.Object{&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example" + constants.SuffixTLSServer, Namespace: "default"}, Data: map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")}}},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    reasonReady,
			wantMessageIn: "provisioned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			reconciler := &OpenBaoClusterReconciler{Client: builder.Build()}

			reconciler.setTLSReadyCondition(context.Background(), tt.cluster)
			cond := meta.FindStatusCondition(tt.cluster.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			if cond == nil {
				t.Fatal("expected TLSReady condition")
			}
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("TLSReady condition = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
			}
			if tt.wantMessageIn != "" && !strings.Contains(cond.Message, tt.wantMessageIn) {
				t.Fatalf("message = %q, want substring %q", cond.Message, tt.wantMessageIn)
			}
		})
	}
}

func TestBuildSealedConditionAndApplyHelpers(t *testing.T) {
	t.Parallel()

	t.Run("sealed condition handles present and absent labels", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			sealed     bool
			present    bool
			wantStatus metav1.ConditionStatus
			wantReason string
		}{
			{name: "unknown", present: false, wantStatus: metav1.ConditionUnknown, wantReason: reasonUnknown},
			{name: "sealed", sealed: true, present: true, wantStatus: metav1.ConditionTrue, wantReason: "Sealed"},
			{name: "unsealed", sealed: false, present: true, wantStatus: metav1.ConditionFalse, wantReason: "Unsealed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cond := buildSealedCondition(tt.sealed, tt.present)
				if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
					t.Fatalf("buildSealedCondition() = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
				}
			})
		}
	})

	t.Run("applyAllConditions populates core conditions and security risk", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
		cluster.Spec.WorkloadHardening = &openbaov1alpha1.WorkloadHardeningConfig{AppArmorEnabled: true}
		state := &clusterState{
			ReadyReplicas:            1,
			Available:                true,
			Initialized:              true,
			InitializedKnown:         true,
			Sealed:                   false,
			SealedKnown:              true,
			LeaderCount:              1,
			LeaderName:               "example-0",
			BackupInProgress:         true,
			BackupJobName:            "backup-job",
			DataPVCCount:             1,
			DataPVCStorageClassNames: []string{"fast"},
			StatefulSet: &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					Conditions: []appsv1.StatefulSetCondition{{
						Type:    "ReplicaFailure",
						Message: "AppArmor profile rejected by node",
					}},
				},
			},
		}
		admissionStatus := &admission.Status{OverallReady: false}
		now := metav1.Now()

		applyAllConditions(cluster, state, admissionStatus, now)

		for _, conditionType := range []openbaov1alpha1.ConditionType{
			openbaov1alpha1.ConditionOpenBaoInitialized,
			openbaov1alpha1.ConditionOpenBaoSealed,
			openbaov1alpha1.ConditionOpenBaoLeader,
			openbaov1alpha1.ConditionAvailable,
			openbaov1alpha1.ConditionDegraded,
			openbaov1alpha1.ConditionUpgrading,
			openbaov1alpha1.ConditionBackingUp,
			openbaov1alpha1.ConditionStorageConfigured,
			openbaov1alpha1.ConditionEtcdEncryptionWarning,
			openbaov1alpha1.ConditionSecurityRisk,
			openbaov1alpha1.ConditionProductionReady,
			openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch,
		} {
			if cond := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType)); cond == nil {
				t.Fatalf("expected condition %s", conditionType)
			}
		}
		nodeMismatch := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch))
		if nodeMismatch == nil || nodeMismatch.Status != metav1.ConditionTrue {
			t.Fatalf("node mismatch condition = %#v, want true", nodeMismatch)
		}
	})

	t.Run("applyNodeSecurityCapabilityMismatchCondition removes condition when apparmor disabled", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Status.Conditions = []metav1.Condition{{
			Type:   string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch),
			Status: metav1.ConditionTrue,
		}}

		applyNodeSecurityCapabilityMismatchCondition(cluster, &clusterState{}, cluster.Generation, metav1.Now())
		if cond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch)); cond != nil {
			t.Fatalf("expected node mismatch condition to be removed, got %#v", cond)
		}
	})
}
