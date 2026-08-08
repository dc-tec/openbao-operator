package networking

import (
	"context"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

const (
	gatewayFeatureHTTPRoute        gatewayv1.FeatureName = "HTTPRoute"
	gatewayFeatureTLSRoute         gatewayv1.FeatureName = "TLSRoute"
	gatewayFeatureBackendTLSPolicy gatewayv1.FeatureName = "BackendTLSPolicy"
)

// ValidateGatewayIntegration evaluates the operator-known Gateway API contract
// for the selected Gateway mode. It validates the referenced Gateway/GatewayClass,
// listener compatibility, advertised feature support, controller status, and
// the current operator-managed Route attachment.
func (m *Manager) ValidateGatewayIntegration(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		return nil
	}

	gatewayNamespace := cluster.Spec.Gateway.GatewayRef.Namespace
	if strings.TrimSpace(gatewayNamespace) == "" {
		gatewayNamespace = cluster.Namespace
	}

	gatewayName := strings.TrimSpace(cluster.Spec.Gateway.GatewayRef.Name)
	if gatewayName == "" {
		return fmt.Errorf("%w: spec.gateway.gatewayRef.name must be set when spec.gateway.enabled=true", ErrGatewayReferenceMissing)
	}

	gateway := &gatewayv1.Gateway{}
	if err := m.reader.Get(ctx, types.NamespacedName{Namespace: gatewayNamespace, Name: gatewayName}, gateway); err != nil {
		if operatorerrors.IsCRDMissingError(err) {
			return ErrGatewayAPIMissing
		}
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: referenced Gateway %s/%s was not found", ErrGatewayReferenceMissing, gatewayNamespace, gatewayName)
		}
		if apierrors.IsForbidden(err) {
			return fmt.Errorf(
				"%w: cannot verify referenced Gateway %s/%s because the operator cannot read it: %v",
				ErrGatewayCapabilitiesUnknown,
				gatewayNamespace,
				gatewayName,
				err,
			)
		}
		return fmt.Errorf("failed to get referenced Gateway %s/%s: %w", gatewayNamespace, gatewayName, err)
	}

	if err := validateGatewayListeners(cluster, gateway); err != nil {
		return err
	}

	gatewayClassName := strings.TrimSpace(string(gateway.Spec.GatewayClassName))
	if gatewayClassName == "" {
		return fmt.Errorf("%w: Gateway %s/%s does not set spec.gatewayClassName", ErrGatewayClassMissing, gatewayNamespace, gatewayName)
	}

	gatewayClass := &gatewayv1.GatewayClass{}
	if err := m.reader.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gatewayClass); err != nil {
		if operatorerrors.IsCRDMissingError(err) {
			return ErrGatewayAPIMissing
		}
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: referenced GatewayClass %q was not found", ErrGatewayClassMissing, gatewayClassName)
		}
		if apierrors.IsForbidden(err) {
			return fmt.Errorf(
				"%w: cannot verify referenced GatewayClass %q because the operator cannot read it: %v",
				ErrGatewayCapabilitiesUnknown,
				gatewayClassName,
				err,
			)
		}
		return fmt.Errorf("failed to get referenced GatewayClass %q: %w", gatewayClassName, err)
	}

	if err := validateGatewayClassAccepted(gatewayClass); err != nil {
		return err
	}
	if err := validateGatewayClassSupportedVersion(gatewayClass); err != nil {
		return err
	}
	if err := validateGatewayClassFeatures(cluster, gatewayClass); err != nil {
		return err
	}
	if err := validateGatewayProgrammed(gateway); err != nil {
		return err
	}
	if err := m.validateManagedRouteAttachment(ctx, cluster, gatewayClass); err != nil {
		return err
	}

	return nil
}

func (m *Manager) validateManagedRouteAttachment(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	gatewayClass *gatewayv1.GatewayClass,
) error {
	if cluster.Spec.Gateway.TLSPassthrough {
		route := &gatewayv1.TLSRoute{}
		name := types.NamespacedName{Namespace: cluster.Namespace, Name: tlsRouteName(cluster)}
		if err := m.reader.Get(ctx, name, route); err != nil {
			return classifyManagedRouteReadError(err, "TLSRoute", name)
		}
		return validateRouteParentStatus(cluster, gatewayClass, "TLSRoute", route.Generation, route.Status.Parents)
	}

	route := &gatewayv1.HTTPRoute{}
	name := types.NamespacedName{Namespace: cluster.Namespace, Name: httpRouteName(cluster)}
	if err := m.reader.Get(ctx, name, route); err != nil {
		return classifyManagedRouteReadError(err, "HTTPRoute", name)
	}
	return validateRouteParentStatus(cluster, gatewayClass, "HTTPRoute", route.Generation, route.Status.Parents)
}

func classifyManagedRouteReadError(err error, kind string, name types.NamespacedName) error {
	if operatorerrors.IsCRDMissingError(err) {
		return ErrGatewayAPIMissing
	}
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%w: operator-managed %s %s/%s does not exist yet", ErrGatewayRoutePending, kind, name.Namespace, name.Name)
	}
	if apierrors.IsForbidden(err) {
		return fmt.Errorf(
			"%w: cannot verify operator-managed %s %s/%s because the operator cannot read it: %v",
			ErrGatewayCapabilitiesUnknown,
			kind,
			name.Namespace,
			name.Name,
			err,
		)
	}
	return fmt.Errorf("failed to get operator-managed %s %s/%s: %w", kind, name.Namespace, name.Name, err)
}

func validateRouteParentStatus(
	cluster *openbaov1alpha1.OpenBaoCluster,
	gatewayClass *gatewayv1.GatewayClass,
	routeKind string,
	routeGeneration int64,
	parents []gatewayv1.RouteParentStatus,
) error {
	parent := findManagedRouteParentStatus(cluster, gatewayClass, parents)
	routeName := httpRouteName(cluster)
	if cluster.Spec.Gateway.TLSPassthrough {
		routeName = tlsRouteName(cluster)
	}
	if parent == nil {
		return fmt.Errorf(
			"%w: operator-managed %s %s/%s has not reported status for the configured Gateway parent",
			ErrGatewayRoutePending,
			routeKind,
			cluster.Namespace,
			routeName,
		)
	}

	accepted := meta.FindStatusCondition(parent.Conditions, string(gatewayv1.RouteConditionAccepted))
	resolvedRefs := meta.FindStatusCondition(parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs))
	if conditionIsCurrentFalse(accepted, routeGeneration) {
		return fmt.Errorf(
			"%w: operator-managed %s %s/%s was rejected by the configured Gateway parent (%s: %s)",
			ErrGatewayRouteNotAccepted,
			routeKind,
			cluster.Namespace,
			routeName,
			accepted.Reason,
			accepted.Message,
		)
	}
	if conditionIsCurrentFalse(resolvedRefs, routeGeneration) {
		return fmt.Errorf(
			"%w: operator-managed %s %s/%s has unresolved references for the configured Gateway parent (%s: %s)",
			ErrGatewayRouteReferencesUnresolved,
			routeKind,
			cluster.Namespace,
			routeName,
			resolvedRefs.Reason,
			resolvedRefs.Message,
		)
	}
	if !conditionIsCurrentTrue(accepted, routeGeneration) || !conditionIsCurrentTrue(resolvedRefs, routeGeneration) {
		return fmt.Errorf(
			"%w: operator-managed %s %s/%s has not reported current Accepted=True and ResolvedRefs=True conditions for the configured Gateway parent",
			ErrGatewayRoutePending,
			routeKind,
			cluster.Namespace,
			routeName,
		)
	}

	return nil
}

func findManagedRouteParentStatus(
	cluster *openbaov1alpha1.OpenBaoCluster,
	gatewayClass *gatewayv1.GatewayClass,
	parents []gatewayv1.RouteParentStatus,
) *gatewayv1.RouteParentStatus {
	gatewayNamespace := strings.TrimSpace(cluster.Spec.Gateway.GatewayRef.Namespace)
	if gatewayNamespace == "" {
		gatewayNamespace = cluster.Namespace
	}
	gatewayName := strings.TrimSpace(cluster.Spec.Gateway.GatewayRef.Name)
	listenerName := strings.TrimSpace(cluster.Spec.Gateway.ListenerName)

	for i := range parents {
		parent := &parents[i]
		if parent.ControllerName != gatewayClass.Spec.ControllerName {
			continue
		}
		if !gatewayParentReferenceMatches(parent.ParentRef, cluster.Namespace, gatewayNamespace, gatewayName, listenerName) {
			continue
		}
		return parent
	}

	return nil
}

func gatewayParentReferenceMatches(
	ref gatewayv1.ParentReference,
	routeNamespace string,
	gatewayNamespace string,
	gatewayName string,
	listenerName string,
) bool {
	group := gatewayv1.GroupVersion.Group
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := "Gateway"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	namespace := routeNamespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	sectionName := ""
	if ref.SectionName != nil {
		sectionName = string(*ref.SectionName)
	}

	return group == gatewayv1.GroupVersion.Group &&
		kind == "Gateway" &&
		string(ref.Name) == gatewayName &&
		namespace == gatewayNamespace &&
		sectionName == listenerName &&
		ref.Port == nil
}

func conditionIsCurrentFalse(condition *metav1.Condition, generation int64) bool {
	return condition != nil && condition.ObservedGeneration == generation && condition.Status == metav1.ConditionFalse
}

func conditionIsCurrentTrue(condition *metav1.Condition, generation int64) bool {
	return condition != nil && condition.ObservedGeneration == generation && condition.Status == metav1.ConditionTrue
}

func validateGatewayListeners(cluster *openbaov1alpha1.OpenBaoCluster, gateway *gatewayv1.Gateway) error {
	if cluster == nil || cluster.Spec.Gateway == nil || gateway == nil {
		return nil
	}

	listeners := gateway.Spec.Listeners
	listenerName := strings.TrimSpace(cluster.Spec.Gateway.ListenerName)
	if listenerName != "" {
		filtered := make([]gatewayv1.Listener, 0, len(listeners))
		for i := range listeners {
			if string(listeners[i].Name) == listenerName {
				filtered = append(filtered, listeners[i])
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf(
				"%w: referenced Gateway %s/%s does not have listener %q",
				ErrGatewayListenerIncompatible,
				gateway.Namespace,
				gateway.Name,
				listenerName,
			)
		}
		listeners = filtered
	}

	for i := range listeners {
		if gatewayListenerCompatible(cluster, listeners[i]) {
			return nil
		}
	}

	modeDescription := "an HTTP/HTTPS listener for HTTPRoute termination"
	if cluster.Spec.Gateway.TLSPassthrough {
		modeDescription = "a TLS listener in Passthrough mode for TLSRoute"
	}

	return fmt.Errorf(
		"%w: referenced Gateway %s/%s does not expose %s",
		ErrGatewayListenerIncompatible,
		gateway.Namespace,
		gateway.Name,
		modeDescription,
	)
}

func gatewayListenerCompatible(cluster *openbaov1alpha1.OpenBaoCluster, listener gatewayv1.Listener) bool {
	if cluster == nil || cluster.Spec.Gateway == nil {
		return false
	}

	if cluster.Spec.Gateway.TLSPassthrough {
		return listener.Protocol == gatewayv1.TLSProtocolType &&
			listener.TLS != nil &&
			listener.TLS.Mode != nil &&
			*listener.TLS.Mode == gatewayv1.TLSModePassthrough
	}

	return listener.Protocol == gatewayv1.HTTPProtocolType || listener.Protocol == gatewayv1.HTTPSProtocolType
}

func validateGatewayClassAccepted(gatewayClass *gatewayv1.GatewayClass) error {
	accepted := meta.FindStatusCondition(gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted))
	if accepted == nil || accepted.Status == metav1.ConditionUnknown {
		return fmt.Errorf("%w: GatewayClass %q has not yet reported Accepted=True", ErrGatewayClassPending, gatewayClass.Name)
	}
	if accepted.Status == metav1.ConditionFalse {
		return fmt.Errorf("%w: GatewayClass %q is not accepted (%s: %s)", ErrGatewayClassNotAccepted, gatewayClass.Name, accepted.Reason, accepted.Message)
	}
	return nil
}

func validateGatewayClassSupportedVersion(gatewayClass *gatewayv1.GatewayClass) error {
	supportedVersion := meta.FindStatusCondition(gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusSupportedVersion))
	if supportedVersion == nil {
		return nil
	}
	if supportedVersion.Status == metav1.ConditionUnknown {
		return fmt.Errorf("%w: GatewayClass %q has not yet reported SupportedVersion=True", ErrGatewayClassPending, gatewayClass.Name)
	}
	if supportedVersion.Status == metav1.ConditionFalse {
		return fmt.Errorf("%w: GatewayClass %q does not support the installed Gateway API version (%s: %s)", ErrGatewayVersionUnsupported, gatewayClass.Name, supportedVersion.Reason, supportedVersion.Message)
	}
	return nil
}

func validateGatewayClassFeatures(cluster *openbaov1alpha1.OpenBaoCluster, gatewayClass *gatewayv1.GatewayClass) error {
	required := requiredGatewayFeatures(cluster)
	if len(required) == 0 {
		return nil
	}

	if len(gatewayClass.Status.SupportedFeatures) == 0 {
		return fmt.Errorf(
			"%w: GatewayClass %q does not publish status.supportedFeatures, so the operator cannot verify support for %s",
			ErrGatewayCapabilitiesUnknown,
			gatewayClass.Name,
			strings.Join(featureNames(required), ", "),
		)
	}

	supported := make([]gatewayv1.FeatureName, 0, len(gatewayClass.Status.SupportedFeatures))
	for _, feature := range gatewayClass.Status.SupportedFeatures {
		supported = append(supported, feature.Name)
	}

	missing := make([]gatewayv1.FeatureName, 0, len(required))
	for _, feature := range required {
		if !slices.Contains(supported, feature) {
			missing = append(missing, feature)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"%w: GatewayClass %q does not advertise support for %s",
			ErrGatewayFeatureUnsupported,
			gatewayClass.Name,
			strings.Join(featureNames(missing), ", "),
		)
	}

	return nil
}

func validateGatewayProgrammed(gateway *gatewayv1.Gateway) error {
	programmed := meta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed))
	if programmed == nil || programmed.Status == metav1.ConditionUnknown {
		return fmt.Errorf("%w: Gateway %s/%s has not yet reported Programmed=True", ErrGatewayProgrammingPending, gateway.Namespace, gateway.Name)
	}
	if programmed.Status == metav1.ConditionFalse {
		return fmt.Errorf("%w: Gateway %s/%s is not programmed (%s: %s)", ErrGatewayNotProgrammed, gateway.Namespace, gateway.Name, programmed.Reason, programmed.Message)
	}
	return nil
}

func requiredGatewayFeatures(cluster *openbaov1alpha1.OpenBaoCluster) []gatewayv1.FeatureName {
	if cluster == nil || cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		return nil
	}

	required := []gatewayv1.FeatureName{}
	if cluster.Spec.Gateway.TLSPassthrough {
		required = append(required, gatewayFeatureTLSRoute)
	} else {
		required = append(required, gatewayFeatureHTTPRoute)
		if gatewayBackendTLSEnabled(cluster) {
			required = append(required, gatewayFeatureBackendTLSPolicy)
		}
	}

	return required
}

func gatewayBackendTLSEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled || cluster.Spec.Gateway.TLSPassthrough {
		return false
	}
	if !cluster.Spec.TLS.Enabled {
		return false
	}

	if cluster.Spec.Gateway.BackendTLS != nil && cluster.Spec.Gateway.BackendTLS.Enabled != nil {
		return *cluster.Spec.Gateway.BackendTLS.Enabled
	}

	return true
}

func featureNames(features []gatewayv1.FeatureName) []string {
	names := make([]string, 0, len(features))
	for _, feature := range features {
		names = append(names, string(feature))
	}
	return names
}
