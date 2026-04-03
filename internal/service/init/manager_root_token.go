package init

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

func (m *Manager) ensureRootTokenSecretPresent(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	secretName := cluster.Name + constants.SuffixRootToken
	_, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if apierrors.IsNotFound(err) {
		return operatorerrors.WrapTransientKubernetesAPI(
			fmt.Errorf("root token Secret %s/%s not found while cluster is already initialized: %w", cluster.Namespace, secretName, err),
		)
	}
	if apierrors.IsForbidden(err) || apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsInternalError(err) {
		return operatorerrors.WrapTransientKubernetesAPI(
			fmt.Errorf("failed to read root token Secret %s/%s while cluster is already initialized: %w", cluster.Namespace, secretName, err),
		)
	}

	return fmt.Errorf("failed to read root token Secret %s/%s while cluster is already initialized: %w", cluster.Namespace, secretName, err)
}

// preflightRootTokenStorage ensures that root token Secret writes are permitted before we
// call the OpenBao init API. The init API returns the root token only once; if we cannot
// persist it, we cannot recover it later.
//
// This uses a DryRun create of the Secret to validate RBAC and admission policies without
// leaving artifacts behind.
func (m *Manager) preflightRootTokenStorage(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}

	secret := buildRootTokenSecret(cluster, "dry-run")
	secretName := secret.Name

	_, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Create(ctx, secret, metav1.CreateOptions{
		DryRun: []string{metav1.DryRunAll},
	})
	if err == nil || apierrors.IsAlreadyExists(err) {
		return nil
	}
	if apierrors.IsForbidden(err) {
		return operatorerrors.WrapTransientKubernetesAPI(
			fmt.Errorf("forbidden to create root token Secret %s/%s (dry-run): %w", cluster.Namespace, secretName, err),
		)
	}
	return fmt.Errorf("failed to dry-run create root token Secret %s/%s: %w", cluster.Namespace, secretName, err)
}

func (m *Manager) storeRootToken(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, rootToken string) error {
	if strings.TrimSpace(rootToken) == "" {
		return nil
	}

	secretsClient := m.clientset.CoreV1().Secrets(cluster.Namespace)
	secretName := cluster.Name + constants.SuffixRootToken
	desired := buildRootTokenSecret(cluster, rootToken)

	storeCtx, cancel := context.WithTimeout(ctx, rootTokenStoreTimeout)
	defer cancel()

	var (
		createdOrExists bool
		alreadyExists   bool
		lastErr         error
	)
	backoff := wait.Backoff{
		Duration: 100 * time.Millisecond,
		Factor:   1.7,
		Jitter:   0.2,
		Steps:    1000,
	}
	err := wait.ExponentialBackoffWithContext(storeCtx, backoff, func(ctx context.Context) (bool, error) {
		_, err := secretsClient.Create(ctx, desired, metav1.CreateOptions{})
		if err == nil {
			createdOrExists = true
			return true, nil
		}
		if apierrors.IsAlreadyExists(err) {
			createdOrExists = true
			alreadyExists = true
			return true, nil
		}
		lastErr = err

		if apierrors.IsForbidden(err) {
			return false, operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("failed to create root token Secret %s/%s: %w", cluster.Namespace, secretName, err),
			)
		}

		if operatorerrors.IsTransientKubernetesAPI(err) || operatorerrors.IsTransientConnection(err) ||
			apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsInternalError(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to create root token Secret %s/%s: %w", cluster.Namespace, secretName, err)
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if lastErr == nil {
				lastErr = err
			}
			return operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("timed out creating root token Secret %s/%s: %w", cluster.Namespace, secretName, lastErr),
			)
		}
		return err
	}
	if createdOrExists && !alreadyExists {
		return nil
	}

	existing, err := secretsClient.Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil
		}
		return fmt.Errorf("failed to get root token Secret %s/%s: %w", cluster.Namespace, secretName, err)
	}

	secretLabels := desired.Labels
	ownerRef := desired.OwnerReferences[0]
	immutable := desired.Immutable != nil && *desired.Immutable

	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for k, v := range secretLabels {
		existing.Labels[k] = v
	}

	hasOwnerRef := false
	for _, ref := range existing.OwnerReferences {
		if ref.UID == cluster.UID {
			hasOwnerRef = true
			break
		}
	}
	if !hasOwnerRef {
		existing.OwnerReferences = append(existing.OwnerReferences, ownerRef)
	}

	if existing.Immutable == nil || *existing.Immutable != immutable {
		existing.Immutable = &immutable
	}

	if _, updateErr := secretsClient.Update(ctx, existing, metav1.UpdateOptions{}); updateErr != nil {
		if apierrors.IsForbidden(updateErr) {
			return nil
		}
		return fmt.Errorf("failed to update root token Secret %s/%s: %w", cluster.Namespace, secretName, updateErr)
	}

	return nil
}

func buildRootTokenSecret(cluster *openbaov1alpha1.OpenBaoCluster, token string) *corev1.Secret {
	secretName := cluster.Name + constants.SuffixRootToken
	ownerRef := metav1.NewControllerRef(cluster, openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"))

	immutable := true
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:    cluster.Name,
				constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster: cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Type:      corev1.SecretTypeOpaque,
		Immutable: &immutable,
		Data: map[string][]byte{
			rootTokenSecretKey: []byte(token),
		},
	}
}
