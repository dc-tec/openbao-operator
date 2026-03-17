package openbaocluster

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func FuzzShouldEmitSecurityWarning(f *testing.F) {
	f.Add("", int64(0))
	f.Add("not-a-time", int64(0))
	f.Add(time.Now().UTC().Add(-securityWarningInterval-time.Minute).Format(time.RFC3339Nano), int64(0))

	f.Fuzz(func(t *testing.T, raw string, deltaSeconds int64) {
		now := time.Unix(1_700_000_000, 0).UTC()
		annotations := map[string]string{
			annotationLastDevelopmentWarning: raw,
		}
		if deltaSeconds%4 == 0 {
			annotations = nil
		} else if deltaSeconds%4 == 1 {
			annotations = map[string]string{}
		} else if deltaSeconds%4 == 2 {
			annotations = map[string]string{
				annotationLastDevelopmentWarning: now.Add(time.Duration(deltaSeconds%10) * time.Minute).Format(time.RFC3339Nano),
			}
		}

		_ = shouldEmitSecurityWarning(annotations, annotationLastDevelopmentWarning, now)
	})
}

func FuzzEmitSecurityWarningEvents(f *testing.F) {
	f.Add(uint8(0), uint8(0), true, "", "")
	f.Add(uint8(1), uint8(1), false, time.Now().UTC().Add(-2*securityWarningInterval).Format(time.RFC3339Nano), "")

	f.Fuzz(func(t *testing.T, profileSeed, unsealSeed uint8, selfInitEnabled bool, existingTimestamp, otherAnnotation string) {
		scheme := newOpenBaoClusterTestScheme(t)
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Name = sanitizeClusterToken(otherAnnotation, "example")
		cluster.Spec.Profile = fuzzProfile(profileSeed)
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: fuzzUnsealType(unsealSeed),
		}
		cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: selfInitEnabled}
		cluster.Annotations = map[string]string{
			annotationLastDevelopmentWarning: strings.TrimSpace(existingTimestamp),
			"example.com/other":              sanitizeMessage(otherAnnotation, "note"),
		}

		recorder := events.NewFakeRecorder(16)
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

		updated := &openbaov1alpha1.OpenBaoCluster{}
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), updated); err != nil {
			t.Fatalf("failed to reload updated cluster: %v", err)
		}
		if updated.Annotations == nil {
			t.Fatalf("expected annotations map to be preserved")
		}
	})
}
