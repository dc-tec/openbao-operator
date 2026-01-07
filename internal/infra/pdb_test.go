package infra

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestEnsurePodDisruptionBudget(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("pdb-test", "default")
	cluster.Spec.Replicas = 3

	ctx := context.Background()

	if err := manager.ensurePodDisruptionBudget(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensurePodDisruptionBudget() error = %v", err)
	}

	pdb := &policyv1.PodDisruptionBudget{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pdbName(cluster),
	}, pdb)
	if err != nil {
		t.Fatalf("expected PDB to exist: %v", err)
	}

	// Verify name
	expectedName := cluster.Name + "-pdb"
	if pdb.Name != expectedName {
		t.Errorf("expected PDB name %q, got %q", expectedName, pdb.Name)
	}

	// Verify MaxUnavailable is 1
	if pdb.Spec.MaxUnavailable == nil {
		t.Fatal("expected MaxUnavailable to be set")
	}
	if pdb.Spec.MaxUnavailable.IntValue() != 1 {
		t.Errorf("expected MaxUnavailable=1, got %d", pdb.Spec.MaxUnavailable.IntValue())
	}

	// Verify selector has correct labels
	if pdb.Spec.Selector == nil {
		t.Fatal("expected PDB Selector to be set")
	}
	expectedLabels := map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}
	for key, expectedVal := range expectedLabels {
		if pdb.Spec.Selector.MatchLabels[key] != expectedVal {
			t.Errorf("expected PDB selector label %s=%s, got %s", key, expectedVal, pdb.Spec.Selector.MatchLabels[key])
		}
	}
}

func TestEnsurePodDisruptionBudgetExcludesJobPods(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("pdb-exclude-jobs", "default")
	cluster.Spec.Replicas = 3

	ctx := context.Background()

	if err := manager.ensurePodDisruptionBudget(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensurePodDisruptionBudget() error = %v", err)
	}

	pdb := &policyv1.PodDisruptionBudget{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pdbName(cluster),
	}, pdb)
	if err != nil {
		t.Fatalf("expected PDB to exist: %v", err)
	}

	// Verify MatchExpressions excludes Job pods (backup, restore, upgrade-snapshot)
	if pdb.Spec.Selector == nil {
		t.Fatal("expected PDB Selector to be set")
	}
	if len(pdb.Spec.Selector.MatchExpressions) == 0 {
		t.Fatal("expected PDB Selector to have MatchExpressions for excluding Job pods")
	}

	// Find the expression that excludes Job components
	foundExclusion := false
	for _, expr := range pdb.Spec.Selector.MatchExpressions {
		if expr.Key == constants.LabelOpenBaoComponent &&
			expr.Operator == metav1.LabelSelectorOpNotIn {
			foundExclusion = true

			// Verify all Job components are excluded
			expectedValues := map[string]bool{
				"backup":                      false,
				"restore":                     false,
				"upgrade":                     false,
				"upgrade-snapshot":            false,
				"validation-hook":             false,
				"post-switch-validation-hook": false,
			}
			for _, v := range expr.Values {
				if _, ok := expectedValues[v]; ok {
					expectedValues[v] = true
				}
			}
			for val, found := range expectedValues {
				if !found {
					t.Errorf("expected MatchExpressions to exclude %q component", val)
				}
			}
			break
		}
	}
	if !foundExclusion {
		t.Error("expected MatchExpressions to contain NotIn expression for openbao.org/component")
	}
}

func TestEnsurePodDisruptionBudgetSkipsSingleReplica(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("pdb-single", "default")
	cluster.Spec.Replicas = 1

	ctx := context.Background()

	if err := manager.ensurePodDisruptionBudget(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensurePodDisruptionBudget() error = %v", err)
	}

	// PDB should NOT be created for single-replica clusters
	pdb := &policyv1.PodDisruptionBudget{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pdbName(cluster),
	}, pdb)
	if err == nil {
		t.Fatal("expected PDB to NOT be created for single-replica cluster")
	}
}

func TestEnsurePodDisruptionBudgetLabels(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("pdb-labels", "default")
	cluster.Spec.Replicas = 5

	ctx := context.Background()

	if err := manager.ensurePodDisruptionBudget(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensurePodDisruptionBudget() error = %v", err)
	}

	pdb := &policyv1.PodDisruptionBudget{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pdbName(cluster),
	}, pdb)
	if err != nil {
		t.Fatalf("expected PDB to exist: %v", err)
	}

	// Verify PDB metadata labels
	expectedLabels := map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}
	for key, expectedVal := range expectedLabels {
		if pdb.Labels[key] != expectedVal {
			t.Errorf("expected PDB label %s=%s, got %s", key, expectedVal, pdb.Labels[key])
		}
	}
}

func TestEnsurePodDisruptionBudgetIsIdempotent(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("pdb-idempotent", "default")
	cluster.Spec.Replicas = 3

	ctx := context.Background()

	// Create PDB first time
	if err := manager.ensurePodDisruptionBudget(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensurePodDisruptionBudget() first call error = %v", err)
	}

	// Get initial resource version
	pdb1 := &policyv1.PodDisruptionBudget{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pdbName(cluster),
	}, pdb1); err != nil {
		t.Fatalf("expected PDB to exist: %v", err)
	}

	// Call again (should be idempotent)
	if err := manager.ensurePodDisruptionBudget(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensurePodDisruptionBudget() second call error = %v", err)
	}

	// Verify PDB still exists
	pdb2 := &policyv1.PodDisruptionBudget{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pdbName(cluster),
	}, pdb2); err != nil {
		t.Fatalf("expected PDB to still exist: %v", err)
	}
}

func TestPDBName(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "default",
		},
	}

	expectedName := "my-cluster-pdb"
	actualName := pdbName(cluster)
	if actualName != expectedName {
		t.Errorf("expected pdbName() = %q, got %q", expectedName, actualName)
	}
}
