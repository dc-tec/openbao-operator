package raftops

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const testLeaderCandidateB = "candidate-b"

func TestFindLeaderPodByLabel(t *testing.T) {
	t.Parallel()

	readyCondition := []corev1.PodCondition{{
		Type:   corev1.PodReady,
		Status: corev1.ConditionTrue,
	}}

	t.Run("finds exactly one active leader label", func(t *testing.T) {
		t.Parallel()

		pods := []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "vault-0", Labels: map[string]string{portopenbao.LabelActive: "true"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "vault-1", Labels: map[string]string{portopenbao.LabelActive: "false"}}},
		}

		got, ok, err := FindLeaderPodByLabel(pods)
		if err != nil {
			t.Fatalf("FindLeaderPodByLabel() error = %v, want nil", err)
		}
		if !ok || got != "vault-0" {
			t.Fatalf("FindLeaderPodByLabel() = %q, %v, want vault-0, true", got, ok)
		}
	})

	t.Run("ignores deleting and invalid labels", func(t *testing.T) {
		t.Parallel()

		now := metav1.Now()
		pods := []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "vault-0",
					DeletionTimestamp: &now,
					Labels:            map[string]string{portopenbao.LabelActive: "true"},
				},
			},
			{ObjectMeta: metav1.ObjectMeta{Name: "vault-1", Labels: map[string]string{portopenbao.LabelActive: "not-a-bool"}}},
		}

		got, ok, err := FindLeaderPodByLabel(pods)
		if err != nil {
			t.Fatalf("FindLeaderPodByLabel() error = %v, want nil", err)
		}
		if ok || got != "" {
			t.Fatalf("FindLeaderPodByLabel() = %q, %v, want empty, false", got, ok)
		}
	})

	t.Run("errors when multiple active labels are present", func(t *testing.T) {
		t.Parallel()

		pods := []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "vault-0", Labels: map[string]string{portopenbao.LabelActive: "true"}}, Status: corev1.PodStatus{Conditions: readyCondition}},
			{ObjectMeta: metav1.ObjectMeta{Name: "vault-1", Labels: map[string]string{portopenbao.LabelActive: "true"}}, Status: corev1.PodStatus{Conditions: readyCondition}},
		}

		_, _, err := FindLeaderPodByLabel(pods)
		if err == nil {
			t.Fatal("FindLeaderPodByLabel() error = nil, want multiple leader error")
		}
	})
}

func TestProbeLeaderPod(t *testing.T) {
	t.Parallel()

	ready := []corev1.PodCondition{{
		Type:   corev1.PodReady,
		Status: corev1.ConditionTrue,
	}}

	t.Run("probes only eligible pods and returns first leader", func(t *testing.T) {
		t.Parallel()

		now := metav1.Now()
		pods := []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "deleting", DeletionTimestamp: &now},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: ready},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "not-ready"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sealed", Labels: map[string]string{portopenbao.LabelSealed: "true"}},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: ready},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "candidate-a"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: ready},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: testLeaderCandidateB},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: ready},
			},
		}

		probed := make([]string, 0, 2)
		got, ok := ProbeLeaderPod(context.Background(), logr.Discard(), pods, func(_ context.Context, pod *corev1.Pod) (bool, error) {
			probed = append(probed, pod.Name)
			return pod.Name == testLeaderCandidateB, nil
		})

		if !ok || got != testLeaderCandidateB {
			t.Fatalf("ProbeLeaderPod() = %q, %v, want %s, true", got, ok, testLeaderCandidateB)
		}
		if len(probed) != 2 || probed[0] != "candidate-a" || probed[1] != testLeaderCandidateB {
			t.Fatalf("probed pods = %v, want [candidate-a %s]", probed, testLeaderCandidateB)
		}
	})

	t.Run("probe errors do not stop later candidates", func(t *testing.T) {
		t.Parallel()

		pods := []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "candidate-a"},
				Status: corev1.PodStatus{
					Phase:      corev1.PodRunning,
					Conditions: ready,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: testLeaderCandidateB},
				Status: corev1.PodStatus{
					Phase:      corev1.PodRunning,
					Conditions: ready,
				},
			},
		}

		got, ok := ProbeLeaderPod(context.Background(), logr.Discard(), pods, func(_ context.Context, pod *corev1.Pod) (bool, error) {
			if pod.Name == "candidate-a" {
				return false, errors.New("temporary failure")
			}
			return true, nil
		})

		if !ok || got != testLeaderCandidateB {
			t.Fatalf("ProbeLeaderPod() = %q, %v, want %s, true", got, ok, testLeaderCandidateB)
		}
	})
}
