// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func bindApprovedLifecycle(
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	policy *openbaov1alpha1.OpenBaoUpgradePolicy,
) ApprovedLifecycle {
	lifecycle := ApprovedLifecycle{}
	if serviceProfile != nil {
		lifecycle.UpgradeStrategy = defaultUpgradeStrategy(serviceProfile.Spec.Lifecycle.UpgradeStrategy)
		lifecycle.PreUpgradeSnapshot = derefBool(serviceProfile.Spec.Lifecycle.PreUpgradeSnapshot)
		if serviceProfile.Spec.Lifecycle.PolicyRef != nil {
			lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: serviceProfile.Spec.Lifecycle.PolicyRef.Name}
		}
	}
	if policy != nil {
		lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: policy.Name}
		lifecycle.BlueGreen = blueGreenConfigFromPolicy(policy.Spec.BlueGreen)
	}
	return lifecycle
}

func blueGreenConfigFromPolicy(policy *openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec) *openbaov1alpha1.BlueGreenConfig {
	if policy == nil {
		return nil
	}
	blueGreen := &openbaov1alpha1.BlueGreenConfig{
		AutoPromote: derefBoolDefaultTrue(policy.AutoPromote),
	}
	if strings.TrimSpace(policy.MinSyncDuration) != "" {
		blueGreen.Verification = &openbaov1alpha1.VerificationConfig{
			MinSyncDuration: strings.TrimSpace(policy.MinSyncDuration),
		}
	}
	if policy.MaxJobFailures != nil {
		value := *policy.MaxJobFailures
		blueGreen.MaxJobFailures = &value
	}
	if policy.AutoRollback != nil {
		blueGreen.AutoRollback = &openbaov1alpha1.AutoRollbackConfig{
			Enabled:             derefBoolDefaultTrue(policy.AutoRollback.Enabled),
			OnJobFailure:        derefBoolDefaultTrue(policy.AutoRollback.OnJobFailure),
			OnValidationFailure: derefBoolDefaultTrue(policy.AutoRollback.OnValidationFailure),
		}
	}
	return blueGreen
}

func defaultUpgradeStrategy(strategy openbaov1alpha1.UpdateStrategyType) openbaov1alpha1.UpdateStrategyType {
	if strategy == "" {
		return openbaov1alpha1.UpdateStrategyRollingUpdate
	}

	return strategy
}
