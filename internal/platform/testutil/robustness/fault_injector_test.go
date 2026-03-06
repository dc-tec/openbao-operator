package robustness

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFaultInjectorTestClient(t *testing.T, inj *Injector, objs ...runtime.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		builder = builder.WithRuntimeObjects(objs...)
	}
	if inj != nil {
		builder = builder.WithInterceptorFuncs(inj.InterceptorFuncs())
	}
	return builder
}

func TestInjector_Once(t *testing.T) {
	t.Parallel()

	expected := errors.New("transient get failure")
	inj := NewInjector(map[Operation]Rule{
		OpGet: Once(expected),
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s1",
			Namespace: "default",
		},
	}
	c := newFaultInjectorTestClient(t, inj, secret).Build()

	target := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, target)
	if !errors.Is(err, expected) {
		t.Fatalf("first Get() error=%v, want %v", err, expected)
	}

	err = c.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, target)
	if err != nil {
		t.Fatalf("second Get() error=%v, want nil", err)
	}
}

func TestInjector_Always(t *testing.T) {
	t.Parallel()

	expected := errors.New("persistent list failure")
	inj := NewInjector(map[Operation]Rule{
		OpList: Always(expected),
	})
	c := newFaultInjectorTestClient(t, inj).Build()

	for i := 0; i < 3; i++ {
		err := c.List(context.Background(), &corev1.SecretList{})
		if !errors.Is(err, expected) {
			t.Fatalf("List() call %d error=%v, want %v", i+1, err, expected)
		}
	}
}

func TestInjector_DisabledRuleDoesNotFail(t *testing.T) {
	t.Parallel()

	inj := NewInjector(map[Operation]Rule{
		OpGet: {Err: errors.New("disabled"), Times: 0},
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s1",
			Namespace: "default",
		},
	}
	c := newFaultInjectorTestClient(t, inj, secret).Build()

	target := &corev1.Secret{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, target); err != nil {
		t.Fatalf("Get() error=%v, want nil", err)
	}
}

func TestStaticClock(t *testing.T) {
	t.Parallel()

	start := metav1.Now().UTC()
	clock := NewStaticClock(start)
	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("Now()=%s, want %s", got, start)
	}

	clock.Advance(2 * time.Second)
	if got := clock.Now(); !got.Equal(start.Add(2 * time.Second)) {
		t.Fatalf("Advance() now=%s, want %s", got, start.Add(2*time.Second))
	}

	next := start.Add(10 * time.Second)
	clock.Set(next)
	if got := clock.Now(); !got.Equal(next) {
		t.Fatalf("Set() now=%s, want %s", got, next)
	}
}
