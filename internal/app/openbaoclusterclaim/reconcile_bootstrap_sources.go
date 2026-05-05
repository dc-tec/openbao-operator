package openbaoclusterclaim

import (
	"context"
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

func (r runtimeReconciler) resolveSameClusterBootstrapInputs(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	catalog *claimcontract.CatalogBundle,
) (claimcontract.SameClusterBootstrapResolvedInputs, result) {
	if localTarget == nil {
		return claimcontract.SameClusterBootstrapResolvedInputs{}, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Same-cluster bootstrap dependency resolution is waiting for the target namespace.",
		}
	}
	if catalog == nil || catalog.BootstrapProfile == nil {
		return claimcontract.SameClusterBootstrapResolvedInputs{}, result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Same-cluster bootstrap dependency resolution does not require additional bootstrap dependency inputs.",
		}
	}

	inputs := claimcontract.SameClusterBootstrapResolvedInputs{
		AuthMethodConfigs:    map[string]claimcontract.ProjectedBootstrapArtifact{},
		PolicyBundleContents: map[string]claimcontract.ProjectedBootstrapArtifact{},
		AuditDeviceSinks:     map[string]claimcontract.ProjectedBootstrapAuditSink{},
	}
	if catalog.BootstrapProfile.Spec.Auth != nil {
		for _, method := range catalog.BootstrapProfile.Spec.Auth.Methods {
			if method.ConfigRef == nil {
				continue
			}
			config, resolution := r.resolveSameClusterAuthMethodConfig(ctx, claim, localTarget, method)
			if !resolution.Valid {
				return claimcontract.SameClusterBootstrapResolvedInputs{}, resolution
			}
			inputs.AuthMethodConfigs[claimcontract.BootstrapAuthMethodIdentity(method.Type, method.Path)] = config
		}
	}
	if catalog.BootstrapProfile.Spec.Policies != nil {
		for _, bundle := range catalog.BootstrapProfile.Spec.Policies.Bundles {
			content, resolution := r.resolveSameClusterPolicyBundleContent(ctx, claim, localTarget, bundle)
			if !resolution.Valid {
				return claimcontract.SameClusterBootstrapResolvedInputs{}, resolution
			}
			inputs.PolicyBundleContents[claimcontract.BootstrapPolicyBundleIdentity(bundle)] = content
		}
	}
	if catalog.BootstrapProfile.Spec.Audit != nil {
		for _, device := range catalog.BootstrapProfile.Spec.Audit.Devices {
			sink, resolution := r.resolveSameClusterAuditSink(ctx, claim, localTarget, device)
			if !resolution.Valid {
				return claimcontract.SameClusterBootstrapResolvedInputs{}, resolution
			}
			inputs.AuditDeviceSinks[claimcontract.BootstrapAuditDeviceIdentity(device)] = sink
		}
	}

	return inputs, result{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Same-cluster bootstrap dependency inputs have been resolved.",
	}
}

func (r runtimeReconciler) resolveSameClusterAuditSink(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	device openbaov1alpha1.OpenBaoBootstrapAuditDeviceSpec,
) (claimcontract.ProjectedBootstrapAuditSink, result) {
	if device.SinkRef == nil {
		return claimcontract.ProjectedBootstrapAuditSink{}, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster bootstrap audit-device projection requires sinkRef-backed sink wiring.",
		}
	}

	artifact, raw, resolution := r.resolveSameClusterProjectedSingleValueArtifact(
		ctx,
		claim,
		localTarget,
		device.SinkRef,
		"bootstrap audit-device sink",
		"audit",
		claimcontract.BootstrapAuditDeviceIdentity(device),
		"sink.json",
	)
	if !resolution.Valid {
		return claimcontract.ProjectedBootstrapAuditSink{}, resolution
	}

	sink := struct {
		Path          string                              `json:"path"`
		Description   string                              `json:"description,omitempty"`
		FileOptions   *openbaov1alpha1.FileAuditOptions   `json:"fileOptions,omitempty"`
		HTTPOptions   *openbaov1alpha1.HTTPAuditOptions   `json:"httpOptions,omitempty"`
		SyslogOptions *openbaov1alpha1.SyslogAuditOptions `json:"syslogOptions,omitempty"`
		SocketOptions *openbaov1alpha1.SocketAuditOptions `json:"socketOptions,omitempty"`
	}{}
	if err := json.Unmarshal([]byte(raw), &sink); err != nil {
		return claimcontract.ProjectedBootstrapAuditSink{}, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Bootstrap audit-device source object must contain valid JSON sink configuration for same-cluster projection.",
		}
	}
	if strings.TrimSpace(sink.Path) == "" {
		return claimcontract.ProjectedBootstrapAuditSink{}, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Bootstrap audit-device sink configuration must define a non-empty path for same-cluster projection.",
		}
	}

	return claimcontract.ProjectedBootstrapAuditSink{
			Artifact:      artifact,
			Path:          strings.Trim(strings.TrimSpace(sink.Path), "/"),
			Description:   sink.Description,
			FileOptions:   sink.FileOptions.DeepCopy(),
			HTTPOptions:   sink.HTTPOptions.DeepCopy(),
			SyslogOptions: sink.SyslogOptions.DeepCopy(),
			SocketOptions: sink.SocketOptions.DeepCopy(),
		}, result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Bootstrap audit-device source has been projected for same-cluster declarative audit configuration.",
		}
}

func (r runtimeReconciler) resolveSameClusterPolicyBundleContent(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	bundle openbaov1alpha1.OpenBaoBootstrapPolicyBundleSpec,
) (claimcontract.ProjectedBootstrapArtifact, result) {
	artifact, _, resolution := r.resolveSameClusterProjectedSingleValueArtifact(
		ctx,
		claim,
		localTarget,
		&bundle.ContentRef,
		"bootstrap policy-bundle content",
		"policy",
		claimcontract.BootstrapPolicyBundleIdentity(bundle),
		"content",
	)
	return artifact, resolution
}

func (r runtimeReconciler) resolveSameClusterAuthMethodConfig(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	method openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec,
) (claimcontract.ProjectedBootstrapArtifact, result) {
	if method.ConfigRef == nil {
		return claimcontract.ProjectedBootstrapArtifact{}, result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Bootstrap auth method does not require a separate config object.",
		}
	}

	return r.resolveSameClusterProjectedStringMapArtifact(
		ctx,
		claim,
		localTarget,
		method.ConfigRef,
		"bootstrap auth-method config",
		"authcfg",
		claimcontract.BootstrapAuthMethodIdentity(method.Type, method.Path),
	)
}

func (r runtimeReconciler) resolveSameClusterProjectedStringMapArtifact(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	ref *openbaov1alpha1.TypedObjectReference,
	purpose string,
	purposeKey string,
	identity string,
) (claimcontract.ProjectedBootstrapArtifact, result) {
	if namespaceResult := validateSameClusterSourceReferenceNamespace(localTarget, ref, purpose); !namespaceResult.Valid {
		return claimcontract.ProjectedBootstrapArtifact{}, namespaceResult
	}
	namespace := resolveSameClusterSourceNamespace(localTarget)
	switch strings.TrimSpace(ref.Kind) {
	case kindConfigMap:
		configMap := &corev1.ConfigMap{}
		key := client.ObjectKey{Namespace: namespace, Name: ref.Name}
		if err := r.client.Get(ctx, key, configMap); err != nil {
			return claimcontract.ProjectedBootstrapArtifact{}, sameClusterSourceLoadResult(err, purpose, kindConfigMap)
		}
		if len(configMap.Data) == 0 {
			return claimcontract.ProjectedBootstrapArtifact{}, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: titleCasePurpose(purpose) + " ConfigMap must contain string data for same-cluster projection.",
			}
		}
		data := copyStringMap(configMap.Data)
		return claimcontract.ProjectedBootstrapArtifact{
				Ref: openbaov1alpha1.TypedObjectReference{
					Kind: kindConfigMap,
					Name: projectedBootstrapArtifactName(claim, purposeKey, identity, projectedBootstrapArtifactIdentity{
						Kind:       kindConfigMap,
						StringData: data,
					}),
				},
				ConfigMapData: data,
			}, result{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: titleCasePurpose(purpose) + " has been resolved into a projected ConfigMap artifact for same-cluster execution.",
			}
	case kindSecret:
		secret := &corev1.Secret{}
		key := client.ObjectKey{Namespace: namespace, Name: ref.Name}
		if err := r.client.Get(ctx, key, secret); err != nil {
			return claimcontract.ProjectedBootstrapArtifact{}, sameClusterSourceLoadResult(err, purpose, kindSecret)
		}
		if len(secret.Data) == 0 {
			return claimcontract.ProjectedBootstrapArtifact{}, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: titleCasePurpose(purpose) + " Secret must contain data for same-cluster projection.",
			}
		}
		data := copySecretData(secret.Data)
		return claimcontract.ProjectedBootstrapArtifact{
				Ref: openbaov1alpha1.TypedObjectReference{
					Kind: kindSecret,
					Name: projectedBootstrapArtifactName(claim, purposeKey, identity, projectedBootstrapArtifactIdentity{
						Kind:       kindSecret,
						SecretData: stringifySecretData(data),
					}),
				},
				SecretData: data,
			}, result{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: titleCasePurpose(purpose) + " has been resolved into a projected Secret artifact for same-cluster execution.",
			}
	default:
		return claimcontract.ProjectedBootstrapArtifact{}, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: titleCasePurpose(purpose) + " currently supports only ConfigMap or Secret source objects.",
		}
	}
}

func (r runtimeReconciler) resolveSameClusterProjectedSingleValueArtifact(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	ref *openbaov1alpha1.TypedObjectReference,
	purpose string,
	purposeKey string,
	identity string,
	dataKey string,
) (claimcontract.ProjectedBootstrapArtifact, string, result) {
	if namespaceResult := validateSameClusterSourceReferenceNamespace(localTarget, ref, purpose); !namespaceResult.Valid {
		return claimcontract.ProjectedBootstrapArtifact{}, "", namespaceResult
	}
	namespace := resolveSameClusterSourceNamespace(localTarget)
	switch strings.TrimSpace(ref.Kind) {
	case kindConfigMap:
		configMap := &corev1.ConfigMap{}
		key := client.ObjectKey{Namespace: namespace, Name: ref.Name}
		if err := r.client.Get(ctx, key, configMap); err != nil {
			return claimcontract.ProjectedBootstrapArtifact{}, "", sameClusterSourceLoadResult(err, purpose, kindConfigMap)
		}
		if len(configMap.Data) != 1 {
			return claimcontract.ProjectedBootstrapArtifact{}, "", result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: titleCasePurpose(purpose) + " ConfigMap must contain exactly one string data entry for same-cluster projection.",
			}
		}
		for _, raw := range configMap.Data {
			if strings.TrimSpace(raw) == "" {
				return claimcontract.ProjectedBootstrapArtifact{}, "", result{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonInvalid,
					Message: titleCasePurpose(purpose) + " ConfigMap content must be non-empty for same-cluster projection.",
				}
			}
			artifactData := map[string]string{dataKey: raw}
			return claimcontract.ProjectedBootstrapArtifact{
					Ref: openbaov1alpha1.TypedObjectReference{
						Kind: kindConfigMap,
						Name: projectedBootstrapArtifactName(claim, purposeKey, identity, projectedBootstrapArtifactIdentity{
							Kind:       kindConfigMap,
							StringData: artifactData,
						}),
					},
					ConfigMapData: artifactData,
				}, raw, result{
					Valid:   true,
					Reason:  openbaov1alpha1.ReasonAccepted,
					Message: titleCasePurpose(purpose) + " has been resolved into a projected ConfigMap artifact for same-cluster execution.",
				}
		}
	case kindSecret:
		secret := &corev1.Secret{}
		key := client.ObjectKey{Namespace: namespace, Name: ref.Name}
		if err := r.client.Get(ctx, key, secret); err != nil {
			return claimcontract.ProjectedBootstrapArtifact{}, "", sameClusterSourceLoadResult(err, purpose, kindSecret)
		}
		if len(secret.Data) != 1 {
			return claimcontract.ProjectedBootstrapArtifact{}, "", result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: titleCasePurpose(purpose) + " Secret must contain exactly one data entry for same-cluster projection.",
			}
		}
		for _, raw := range secret.Data {
			content := string(raw)
			if strings.TrimSpace(content) == "" {
				return claimcontract.ProjectedBootstrapArtifact{}, "", result{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonInvalid,
					Message: titleCasePurpose(purpose) + " Secret content must be non-empty for same-cluster projection.",
				}
			}
			artifactData := map[string][]byte{dataKey: append([]byte(nil), raw...)}
			return claimcontract.ProjectedBootstrapArtifact{
					Ref: openbaov1alpha1.TypedObjectReference{
						Kind: kindSecret,
						Name: projectedBootstrapArtifactName(claim, purposeKey, identity, projectedBootstrapArtifactIdentity{
							Kind:       kindSecret,
							SecretData: stringifySecretData(artifactData),
						}),
					},
					SecretData: artifactData,
				}, content, result{
					Valid:   true,
					Reason:  openbaov1alpha1.ReasonAccepted,
					Message: titleCasePurpose(purpose) + " has been resolved into a projected Secret artifact for same-cluster execution.",
				}
		}
	default:
		return claimcontract.ProjectedBootstrapArtifact{}, "", result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: titleCasePurpose(purpose) + " currently supports only ConfigMap or Secret source objects.",
		}
	}

	return claimcontract.ProjectedBootstrapArtifact{}, "", result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: titleCasePurpose(purpose) + " could not be resolved for same-cluster projection.",
	}
}

func resolveSameClusterSourceNamespace(localTarget *openbaov1alpha1.NamespacedReference) string {
	if localTarget == nil {
		return ""
	}
	return localTarget.Namespace
}

func validateSameClusterSourceReferenceNamespace(
	localTarget *openbaov1alpha1.NamespacedReference,
	ref *openbaov1alpha1.TypedObjectReference,
	purpose string,
) result {
	if localTarget == nil || localTarget.Namespace == "" || ref == nil {
		return result{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if strings.TrimSpace(ref.Namespace) == "" || strings.TrimSpace(ref.Namespace) == localTarget.Namespace {
		return result{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	return result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: titleCasePurpose(purpose) + " must reside in the resolved same-cluster target namespace.",
	}
}

func sameClusterSourceLoadResult(err error, purpose, kind string) result {
	if apierrors.IsNotFound(err) {
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: titleCasePurpose(purpose) + " " + kind + " does not exist yet.",
		}
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: titleCasePurpose(purpose) + " " + kind + " access is waiting for same-cluster tenant secret RBAC to converge.",
		}
	}
	if operatorerrors.IsTransientKubernetesAPI(err) {
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: titleCasePurpose(purpose) + " " + kind + " could not be loaded yet.",
		}
	}
	return result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: titleCasePurpose(purpose) + " " + kind + " could not be loaded for same-cluster projection.",
	}
}

func titleCasePurpose(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) == 1 {
		return strings.ToUpper(value)
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func projectedBootstrapArtifactName(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	purpose string,
	identity string,
	payload projectedBootstrapArtifactIdentity,
) string {
	suffix := strings.TrimPrefix(claimcontract.IdentityHash(struct {
		ClaimNamespace string                             `json:"claimNamespace"`
		ClaimName      string                             `json:"claimName"`
		Purpose        string                             `json:"purpose"`
		Identity       string                             `json:"identity"`
		Payload        projectedBootstrapArtifactIdentity `json:"payload"`
	}{
		ClaimNamespace: claim.Namespace,
		ClaimName:      claim.Name,
		Purpose:        purpose,
		Identity:       identity,
		Payload:        payload,
	}), "sha256:")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	return "claim-bootstrap-" + purpose + "-" + suffix
}

type projectedBootstrapArtifactIdentity struct {
	Kind       string            `json:"kind"`
	StringData map[string]string `json:"stringData,omitempty"`
	SecretData map[string]string `json:"secretData,omitempty"`
}

func copyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copySecretData(values map[string][]byte) map[string][]byte {
	if values == nil {
		return nil
	}
	copy := make(map[string][]byte, len(values))
	for key, value := range values {
		copy[key] = append([]byte(nil), value...)
	}
	return copy
}

func stringifySecretData(values map[string][]byte) map[string]string {
	if values == nil {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = string(value)
	}
	return copy
}
