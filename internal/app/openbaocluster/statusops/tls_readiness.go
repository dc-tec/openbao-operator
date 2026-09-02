package statusops

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
)

// EvaluateTLSReadiness evaluates the TLS assets that the operator can observe.
// The reader is the reconciler's cached Kubernetes API reader.
func EvaluateTLSReadiness(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
) ConditionResult {
	if !cluster.Spec.TLS.Enabled {
		return ConditionResult{
			Status:  metav1.ConditionTrue,
			Reason:  ReasonDisabled,
			Message: "TLS is disabled",
		}
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME {
		return ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness",
		}
	}

	// Check the CA TLS Secret first. Day-2 workflows depend on the cluster trust
	// bundle, not only the leaf certificate.
	caSecret := &corev1.Secret{}
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixTLSCA,
	}, caSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretMissing,
				Message: "CA TLS Secret is not present yet",
			}
		}
		return ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Failed to get CA TLS secret",
		}
	}
	if err := openbaotls.ValidateCABundle(caSecret.Data["ca.crt"]); err != nil {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonTLSSecretInvalid,
			Message: "CA TLS Secret is invalid: " + err.Error(),
		}
	}

	serverSecret := &corev1.Secret{}
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixTLSServer,
	}, serverSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretMissing,
				Message: "Server TLS Secret is not present yet",
			}
		}
		return ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Failed to get TLS secret",
		}
	}

	if tlsMode == openbaov1alpha1.TLSModeExternal {
		if err := openbaotls.ValidateExternalServerSecret(cluster, caSecret, serverSecret); err != nil {
			return ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonTLSSecretInvalid,
				Message: "External TLS assets are invalid: " + err.Error(),
			}
		}
		return ConditionResult{
			Status:  metav1.ConditionTrue,
			Reason:  reasonReady,
			Message: "TLS assets are provisioned and valid",
		}
	}

	if _, err := openbaotls.ValidateServerSecret(serverSecret); err != nil {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonTLSSecretInvalid,
			Message: "Server TLS Secret is invalid: " + err.Error(),
		}
	}
	return ConditionResult{
		Status:  metav1.ConditionTrue,
		Reason:  reasonReady,
		Message: "TLS assets are provisioned",
	}
}
