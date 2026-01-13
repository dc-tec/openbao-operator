package bluegreen

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

type autoRollbackConfig struct {
	Enabled             bool
	OnJobFailure        bool
	OnValidationFailure bool
}

func autoRollbackSettings(cluster *openbaov1alpha1.OpenBaoCluster) autoRollbackConfig {
	// Defaults mirror kubebuilder CRD defaults.
	cfg := autoRollbackConfig{
		Enabled:             true,
		OnJobFailure:        true,
		OnValidationFailure: true,
	}

	if cluster == nil || cluster.Spec.UpdateStrategy.BlueGreen == nil || cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback == nil {
		return cfg
	}

	ar := cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback
	cfg.Enabled = ar.Enabled
	cfg.OnJobFailure = ar.OnJobFailure
	cfg.OnValidationFailure = ar.OnValidationFailure

	return cfg
}
