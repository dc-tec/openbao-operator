package entrypoint

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

const (
	admissionReadinessRefreshInterval = 15 * time.Second
	admissionReadinessTimeout         = 10 * time.Second
	admissionReadinessMaxAge          = 30 * time.Second
)

// AddManagerHealthChecks registers process liveness and cache/admission readiness.
// watchedObjects must contain the types watched by this manager. Registering their
// informers before startup makes cache synchronization independent of leadership.
func AddManagerHealthChecks(ctx context.Context, mgr ctrl.Manager, watchedObjects ...client.Object) error {
	readiness := &managerReadiness{reader: mgr.GetAPIReader()}
	for _, object := range watchedObjects {
		informer, err := mgr.GetCache().GetInformer(ctx, object, cache.BlockUntilSynced(false))
		if err != nil {
			return fmt.Errorf("register readiness informer for %T: %w", object, err)
		}
		readiness.informers = append(readiness.informers, informer)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("register liveness check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", readiness.Check); err != nil {
		return fmt.Errorf("register readiness check: %w", err)
	}
	if err := mgr.Add(readiness); err != nil {
		return fmt.Errorf("register readiness refresh: %w", err)
	}
	return nil
}

type managerReadiness struct {
	reader    client.Reader
	informers []cache.Informer
	running   atomic.Bool

	mu              sync.RWMutex
	admissionStatus *admission.Status
}

// NeedLeaderElection keeps readiness current on idle and standby replicas.
func (r *managerReadiness) NeedLeaderElection() bool { return false }

func (r *managerReadiness) Start(ctx context.Context) error {
	// The manager starts non-leader runnables after its registered caches sync.
	// This does not imply completion of controller warmup or reconciliation.
	r.running.Store(true)
	defer r.running.Store(false)
	ticker := time.NewTicker(admissionReadinessRefreshInterval)
	defer ticker.Stop()
	r.refresh(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.refresh(ctx)
		}
	}
}

func (r *managerReadiness) refresh(ctx context.Context) {
	if admission.UnsafeAdmissionDisabled() {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, admissionReadinessTimeout)
	defer cancel()
	status, err := admission.CheckDependencies(
		checkCtx, r.reader, admission.DefaultDependencies(), admission.DefaultNamePrefixes(),
	)
	if err != nil {
		status = admission.Status{CheckedAt: time.Now()}
	}
	// Keep probe observations separate from mutation-path trackers and their gauge.
	r.mu.Lock()
	r.admissionStatus = &status
	r.mu.Unlock()
}

func (r *managerReadiness) Check(_ *http.Request) error {
	if !r.running.Load() {
		return fmt.Errorf("manager cache synchronization is incomplete or the manager is stopping")
	}
	for _, informer := range r.informers {
		if !informer.HasSynced() || informer.IsStopped() {
			return fmt.Errorf("a watched resource cache has not synchronized or has stopped")
		}
	}
	if admission.UnsafeAdmissionDisabled() {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.admissionStatus == nil {
		return fmt.Errorf("admission dependencies have not been checked")
	}
	age := time.Since(r.admissionStatus.CheckedAt)
	if age < 0 || age >= admissionReadinessMaxAge {
		return fmt.Errorf("admission dependency status is stale")
	}
	if !r.admissionStatus.OverallReady {
		return fmt.Errorf("admission dependencies are not ready")
	}
	return nil
}
