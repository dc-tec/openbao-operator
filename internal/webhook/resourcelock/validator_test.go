package resourcelock

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestResourceLockValidator_Handle_Decisions(t *testing.T) {
	operatorUser := "system:serviceaccount:openbao-operator-system:openbao-operator-controller"
	kubeSystemUser := "system:serviceaccount:kube-system:generic-garbage-collector"
	systemSAUser := "system:serviceaccount:kube-system:foo"
	certManagerUser := "system:serviceaccount:cert-manager:cert-manager"
	ingressUser := "system:serviceaccount:traefik-system:traefik"
	adminGroup := "system:masters"
	nodeUser := "system:node:test-node-1"

	tests := []struct {
		name        string
		meta        *metav1.PartialObjectMetadata
		validator   func() *ResourceLockValidator
		request     admission.Request
		wantAllowed bool
	}{
		{
			name: "unmanaged resource is always allowed",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: "tenant-user"},
				},
			},
			wantAllowed: true,
		},
		{
			name: "operator controller can update managed resource",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.operatorIdentity = operatorUser
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: operatorUser},
				},
			},
			wantAllowed: true,
		},
		{
			name: "kube-system group controller allowed for managed resource",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					UserInfo: authenticationv1.UserInfo{
						Username: kubeSystemUser,
						Groups:   []string{kubeSystemGroup},
					},
				},
			},
			wantAllowed: true,
		},
		{
			name: "system serviceaccount allowlist grants access",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.systemSAUsernames = map[string]struct{}{
					systemSAUser: {},
				}
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					UserInfo:  authenticationv1.UserInfo{Username: systemSAUser},
				},
			},
			wantAllowed: true,
		},
		{
			name: "cert-manager allowlist grants access",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.certManagerSAs = map[string]struct{}{
					certManagerUser: {},
				}
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: certManagerUser},
				},
			},
			wantAllowed: true,
		},
		{
			name: "ingress controller allowed to update status subresource",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.ingressControllerSAs = map[string]struct{}{
					ingressUser: {},
				}
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation:   admissionv1.Update,
					SubResource: "status",
					UserInfo:    authenticationv1.UserInfo{Username: ingressUser},
				},
			},
			wantAllowed: true,
		},
		{
			name: "break-glass admin with maintenance annotation allowed",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.breakGlassAdminGroups = map[string]struct{}{
					adminGroup: {},
				}
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo: authenticationv1.UserInfo{
						Username: "cluster-admin",
						Groups:   []string{adminGroup},
					},
				},
			},
			wantAllowed: true,
		},
		{
			name: "tenant user denied for managed resource without exceptions",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: "tenant-user"},
				},
			},
			wantAllowed: false,
		},
		{
			name: "kubelet node allowed to delete managed pod",
			validator: func() *ResourceLockValidator {
				v := NewValidator(logr.Discard())
				v.decoder = decoder
				return v
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					Operation: admissionv1.Delete,
					UserInfo: authenticationv1.UserInfo{
						Username: nodeUser,
					},
				},
			},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.validator()

			// Build a Secret backing object for managed vs unmanaged cases.
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "obj",
					Namespace: "tenant-a",
				},
			}

			switch tt.name {
			case "unmanaged resource is always allowed":
				// no labels
			case "break-glass admin with maintenance annotation allowed":
				if secret.Annotations == nil {
					secret.Annotations = make(map[string]string)
				}
				secret.Annotations[maintenanceAnnotation] = "true"
				fallthrough
			default:
				if secret.Labels == nil {
					secret.Labels = make(map[string]string)
				}
				secret.Labels[managedByLabelKey] = managedByLabelValue
			}

			raw, err := json.Marshal(secret)
			if err != nil {
				t.Fatalf("failed to marshal secret: %v", err)
			}

			tt.request.Object = runtime.RawExtension{Raw: raw}
			tt.request.OldObject = runtime.RawExtension{Raw: raw}

			resp := v.Handle(context.Background(), tt.request)
			if resp.Allowed != tt.wantAllowed {
				t.Fatalf("Handle() allowed=%v, want %v", resp.Allowed, tt.wantAllowed)
			}
		})
	}
}

func TestResourceLockValidator_AllowsOpenBaoServiceRegistrationPodLabelPatch(t *testing.T) {
	v := NewValidator(logr.Discard())
	v.decoder = decoder

	oldPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbaocluster-full-0",
			Namespace: "openbaocluster-full",
			Labels: map[string]string{
				managedByLabelKey:   managedByLabelValue,
				labelAppInstanceKey: "openbaocluster-full",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "openbaocluster-full-serviceaccount",
		},
	}

	newPod := oldPod.DeepCopy()
	newPod.Labels["openbao-initialized"] = "true"

	oldRaw, err := json.Marshal(oldPod)
	if err != nil {
		t.Fatalf("failed to marshal old pod: %v", err)
	}
	newRaw, err := json.Marshal(newPod)
	if err != nil {
		t.Fatalf("failed to marshal new pod: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Kind: metav1.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			Operation: admissionv1.Update,
			Namespace: "openbaocluster-full",
			Name:      "openbaocluster-full-0",
			UserInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:openbaocluster-full:openbaocluster-full-serviceaccount",
			},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}

	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Handle() allowed=false, want true: %s", resp.Result.Message)
	}
}

func TestResourceLockValidator_DeniesOpenBaoServiceRegistrationWhenNonServiceLabelChanges(t *testing.T) {
	v := NewValidator(logr.Discard())
	v.decoder = decoder

	oldPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbaocluster-full-0",
			Namespace: "openbaocluster-full",
			Labels: map[string]string{
				managedByLabelKey:        managedByLabelValue,
				labelAppInstanceKey:      "openbaocluster-full",
				"app.kubernetes.io/name": "openbao",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "openbaocluster-full-serviceaccount",
		},
	}

	newPod := oldPod.DeepCopy()
	newPod.Labels["app.kubernetes.io/name"] = "tampered"

	oldRaw, err := json.Marshal(oldPod)
	if err != nil {
		t.Fatalf("failed to marshal old pod: %v", err)
	}
	newRaw, err := json.Marshal(newPod)
	if err != nil {
		t.Fatalf("failed to marshal new pod: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Kind: metav1.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			Operation: admissionv1.Update,
			Namespace: "openbaocluster-full",
			Name:      "openbaocluster-full-0",
			UserInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:openbaocluster-full:openbaocluster-full-serviceaccount",
			},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}

	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("Handle() allowed=true, want false")
	}
}
