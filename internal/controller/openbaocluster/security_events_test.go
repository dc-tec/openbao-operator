package openbaocluster

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/client-go/tools/events"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

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
		Client: k8sClient,
		ControllerRuntime: ControllerRuntime{
			Recorder: recorder,
		},
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

func TestEmitSecurityWarningEvents_EmitsAmbientUnsealIdentityNote(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true}
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "awskms",
		AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
			Region:   "eu-central-1",
			KMSKeyID: "alias/openbao",
		},
	}
	cluster.Spec.ServiceAccount = &openbaov1alpha1.ServiceAccountConfig{
		Annotations: map[string]string{
			"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao",
		},
	}
	cluster.Annotations = map[string]string{}

	recorder := events.NewFakeRecorder(10)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	reconciler := &OpenBaoClusterReconciler{
		Client: k8sClient,
		ControllerRuntime: ControllerRuntime{
			Recorder: recorder,
		},
	}

	if err := reconciler.emitSecurityWarningEvents(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("emitSecurityWarningEvents() error = %v", err)
	}

	found := false
	for i := 0; i < 4; i++ {
		select {
		case event := <-recorder.Events:
			if strings.Contains(event, ReasonAmbientUnsealIdentity) {
				found = true
			}
		default:
		}
	}
	if !found {
		t.Fatal("expected ambient unseal identity event to be recorded")
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	if updated.Annotations[annotationLastAmbientUnsealNote] == "" {
		t.Fatalf("expected annotation %q to be persisted", annotationLastAmbientUnsealNote)
	}
}

func TestEmitSecurityWarningEvents_DoesNotEmitAmbientUnsealIdentityForInlineCredentials(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true}
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "awskms",
		AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
			Region:    "eu-central-1",
			KMSKeyID:  "alias/openbao",
			AccessKey: "AKIA...",
			SecretKey: "secret",
		},
	}
	cluster.Annotations = map[string]string{}

	recorder := events.NewFakeRecorder(10)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	reconciler := &OpenBaoClusterReconciler{
		Client: k8sClient,
		ControllerRuntime: ControllerRuntime{
			Recorder: recorder,
		},
	}

	if err := reconciler.emitSecurityWarningEvents(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("emitSecurityWarningEvents() error = %v", err)
	}

	for i := 0; i < 4; i++ {
		select {
		case event := <-recorder.Events:
			if strings.Contains(event, ReasonAmbientUnsealIdentity) {
				t.Fatalf("unexpected ambient unseal identity event: %q", event)
			}
		default:
		}
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	if updated.Annotations[annotationLastAmbientUnsealNote] != "" {
		t.Fatalf("did not expect annotation %q to be persisted", annotationLastAmbientUnsealNote)
	}
}
