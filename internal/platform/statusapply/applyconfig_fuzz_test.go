package statusapply

import (
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func FuzzToApplyConfiguration(f *testing.F) {
	f.Add(false, false, false, "config", "default", "key", "value")
	f.Add(false, true, false, "", "", "", "")
	f.Add(true, false, true, "secret", "ns", "k", "v")

	f.Fuzz(func(t *testing.T, hasGVK, useNilObject, resolverFails bool, name, namespace, dataKey, dataValue string) {
		var obj *corev1.ConfigMap
		if !useNilObject {
			obj = &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sanitizeApplyToken(name, "config"),
					Namespace: sanitizeApplyToken(namespace, "default"),
				},
				Data: map[string]string{
					sanitizeApplyToken(dataKey, "key"): sanitizeApplyText(dataValue, "value"),
				},
			}
			if hasGVK {
				obj.APIVersion = corev1.SchemeGroupVersion.String()
				obj.Kind = "ConfigMap"
			}
		}

		var resolver GVKResolver
		if !hasGVK {
			if resolverFails {
				resolver = applyResolverAdapter{gvk: schema.GroupVersionKind{}, err: errors.New("resolver failed")}
			} else {
				resolver = applyResolverAdapter{gvk: corev1.SchemeGroupVersion.WithKind("ConfigMap")}
			}
		}

		cfg, err := ToApplyConfiguration(obj, resolver)
		if useNilObject {
			if err == nil {
				t.Fatalf("expected nil object to fail")
			}
			return
		}
		if !hasGVK && resolverFails {
			if err == nil {
				t.Fatalf("expected resolver failure when GVK is missing")
			}
			return
		}
		if err != nil {
			t.Fatalf("ToApplyConfiguration() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("expected non-nil apply configuration")
		}
	})
}

type applyResolverAdapter struct {
	gvk schema.GroupVersionKind
	err error
}

func (r applyResolverAdapter) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return r.gvk, r.err
}

func sanitizeApplyToken(input, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func sanitizeApplyText(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}
