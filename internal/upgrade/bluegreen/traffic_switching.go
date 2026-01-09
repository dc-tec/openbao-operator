package bluegreen

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func trafficStrategy(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.BlueGreenTrafficStrategy {
	strategy := openbaov1alpha1.BlueGreenTrafficStrategyServiceSelectors
	if cluster.Spec.UpdateStrategy.BlueGreen != nil {
		if cluster.Spec.UpdateStrategy.BlueGreen.TrafficStrategy != "" {
			strategy = cluster.Spec.UpdateStrategy.BlueGreen.TrafficStrategy
		} else if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
			strategy = openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights
		}
	}
	return strategy
}

type trafficStabilizationObservation struct {
	trafficSwitchedAt *time.Time
	hookResult        *JobResult
	greenHealthy      bool
}

func (m *Manager) observeTrafficStabilization(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	greenRevision string,
	now time.Time,
	autoRollback autoRollbackConfig,
) (trafficStabilizationObservation, error) {
	stabilizationSeconds := autoRollback.StabilizationSeconds

	var trafficSwitchedAt *time.Time
	var hookResult *JobResult
	greenHealthy := true

	if cluster.Status.BlueGreen.TrafficSwitchedTime == nil {
		return trafficStabilizationObservation{
			trafficSwitchedAt: nil,
			hookResult:        nil,
			greenHealthy:      true,
		}, nil
	}

	switchedAt := cluster.Status.BlueGreen.TrafficSwitchedTime.Time
	trafficSwitchedAt = &switchedAt

	requiredDuration := time.Duration(stabilizationSeconds) * time.Second
	elapsed := now.Sub(switchedAt)

	if elapsed >= requiredDuration {
		return trafficStabilizationObservation{
			trafficSwitchedAt: trafficSwitchedAt,
			hookResult:        nil,
			greenHealthy:      true,
		}, nil
	}

	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification.PostSwitchHook != nil {
		hook := cluster.Spec.UpdateStrategy.BlueGreen.Verification.PostSwitchHook
		result, err := m.ensurePostSwitchHookJob(ctx, logger, cluster, hook)
		if err != nil {
			return trafficStabilizationObservation{}, fmt.Errorf("failed to ensure post-switch hook job: %w", err)
		}
		hookResult = result
		if hookResult.Running {
			logger.Info("Post-switch hook job is in progress", "job", hookResult.Name)
		}
		if hookResult.Failed {
			logger.Info("Post-switch hook job failed", "job", hookResult.Name)
		}
		if hookResult.Succeeded {
			logger.Info("Post-switch hook completed successfully", "job", hookResult.Name)
		}
	}

	if autoRollback.Enabled && autoRollback.OnTrafficFailure {
		greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
		if err != nil {
			return trafficStabilizationObservation{}, fmt.Errorf("failed to get Green pods: %w", err)
		}
		for i := range greenPods {
			if !isPodReady(&greenPods[i]) {
				greenHealthy = false
				break
			}
		}
	}

	return trafficStabilizationObservation{
		trafficSwitchedAt: trafficSwitchedAt,
		hookResult:        hookResult,
		greenHealthy:      greenHealthy,
	}, nil
}

func (m *Manager) handleTrafficSwitching(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, greenRevision string, strategy openbaov1alpha1.BlueGreenTrafficStrategy) (phaseOutcome, error) {
	autoRollback := autoRollbackSettings(cluster)
	stabilizationSeconds := autoRollback.StabilizationSeconds

	now := time.Now()

	obs, err := m.observeTrafficStabilization(ctx, logger, cluster, greenRevision, now, autoRollback)
	if err != nil {
		return phaseOutcome{}, err
	}

	var decision trafficSwitchDecision
	switch strategy {
	case openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights:
		decision, err = trafficSwitchGatewayWeightsDecision(
			now,
			cluster.Status.BlueGreen.TrafficStep,
			obs.trafficSwitchedAt,
			stabilizationSeconds,
			autoRollback,
			obs.hookResult,
			obs.greenHealthy,
		)
	default:
		decision, err = trafficSwitchServiceSelectorsDecision(
			now,
			obs.trafficSwitchedAt,
			stabilizationSeconds,
			autoRollback,
			obs.hookResult,
			obs.greenHealthy,
		)
	}
	if err != nil {
		return phaseOutcome{}, err
	}

	if decision.SetTrafficStep {
		cluster.Status.BlueGreen.TrafficStep = decision.TrafficStep
	}
	if decision.SetTrafficSwitchedAt {
		at := metav1.NewTime(decision.TrafficSwitchedAt)
		cluster.Status.BlueGreen.TrafficSwitchedTime = &at
		if strategy != openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights {
			logger.Info("Traffic switched to Green", "greenRevision", greenRevision)
		}
	}

	if strategy == openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights {
		if m.infraManager != nil {
			if err := m.infraManager.EnsureGatewayTrafficResources(ctx, logger, cluster); err != nil {
				return phaseOutcome{}, fmt.Errorf("failed to reconcile Gateway traffic resources: %w", err)
			}
		}

		if decision.SetTrafficStep {
			switch decision.TrafficStep {
			case 1:
				logger.Info("Gateway weighted traffic: starting canary step", "trafficStep", cluster.Status.BlueGreen.TrafficStep)
			case 2:
				logger.Info("Gateway weighted traffic: advancing to mid-step", "trafficStep", cluster.Status.BlueGreen.TrafficStep)
			case 3:
				logger.Info("Gateway weighted traffic: advancing to final step", "trafficStep", cluster.Status.BlueGreen.TrafficStep)
			}
		}
	}

	if decision.Outcome.kind == phaseOutcomeAdvance && decision.Outcome.nextPhase == openbaov1alpha1.PhaseDemotingBlue {
		if strategy == openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights {
			logger.Info("Gateway weighted traffic stabilization complete, proceeding to demote Blue",
				"trafficStep", cluster.Status.BlueGreen.TrafficStep)
		} else {
			logger.Info("Stabilization period complete, proceeding to demote Blue")
		}
	}

	return decision.Outcome, nil
}
