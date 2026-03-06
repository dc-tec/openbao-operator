package robustness

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// Operation identifies a client call that can be fault-injected.
type Operation string

const (
	OpGet    Operation = "get"
	OpList   Operation = "list"
	OpCreate Operation = "create"
	OpDelete Operation = "delete"
	OpUpdate Operation = "update"
	OpPatch  Operation = "patch"
	OpApply  Operation = "apply"
)

// Rule configures how many times an operation should fail.
// Times values:
// - -1: fail forever
// -  0: disabled (never fail)
// - >0: fail exactly N times
type Rule struct {
	Err   error
	Times int
}

// Once fails exactly one call with err.
func Once(err error) Rule {
	return Rule{Err: err, Times: 1}
}

// Always fails every call with err.
func Always(err error) Rule {
	return Rule{Err: err, Times: -1}
}

type ruleState struct {
	err   error
	times int
}

// Injector applies transient or persistent failures to client operations.
type Injector struct {
	mu    sync.Mutex
	rules map[Operation]*ruleState
}

// NewInjector returns a fault injector with immutable operation rules.
func NewInjector(rules map[Operation]Rule) *Injector {
	stored := make(map[Operation]*ruleState, len(rules))
	for op, rule := range rules {
		stored[op] = &ruleState{
			err:   rule.Err,
			times: rule.Times,
		}
	}
	return &Injector{rules: stored}
}

func (i *Injector) shouldFail(op Operation) (error, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	state, ok := i.rules[op]
	if !ok || state == nil || state.err == nil || state.times == 0 {
		return nil, false
	}
	if state.times > 0 {
		state.times--
	}
	return state.err, true
}

// InterceptorFuncs creates controller-runtime interceptor funcs with the configured faults.
func (i *Injector) InterceptorFuncs() interceptor.Funcs {
	if i == nil {
		return interceptor.Funcs{}
	}

	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err, fail := i.shouldFail(OpGet); fail {
				return err
			}
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if err, fail := i.shouldFail(OpList); fail {
				return err
			}
			return c.List(ctx, list, opts...)
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if err, fail := i.shouldFail(OpCreate); fail {
				return err
			}
			return c.Create(ctx, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if err, fail := i.shouldFail(OpDelete); fail {
				return err
			}
			return c.Delete(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if err, fail := i.shouldFail(OpUpdate); fail {
				return err
			}
			return c.Update(ctx, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if err, fail := i.shouldFail(OpPatch); fail {
				return err
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
		Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
			if err, fail := i.shouldFail(OpApply); fail {
				return err
			}
			return c.Apply(ctx, obj, opts...)
		},
	}
}
