package upgrade

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// ReconcileResult carries scheduling and request acknowledgements that the
// application must persist with the resulting status changes, including on error.
type ReconcileResult struct {
	recon.Result
	Acknowledgements RequestAcknowledgements
}

// RequestAcknowledgements contains the exact request tokens handled during a
// reconcile pass. Empty fields leave the corresponding observed status unchanged.
// Keep this intent separate from cluster status: checkpoint read-back replaces
// observed status before the final application write.
type RequestAcknowledgements struct {
	Retry    string
	Promote  string
	Rollback string
}

// IsEmpty reports whether there are no request tokens to acknowledge.
func (a RequestAcknowledgements) IsEmpty() bool {
	return strings.TrimSpace(a.Retry) == "" &&
		strings.TrimSpace(a.Promote) == "" &&
		strings.TrimSpace(a.Rollback) == ""
}

// Merge adds acknowledgements from a later sub-reconciler without clearing
// tokens handled by an earlier one.
func (a *RequestAcknowledgements) Merge(next RequestAcknowledgements) {
	if token := strings.TrimSpace(next.Retry); token != "" {
		a.Retry = token
	}
	if token := strings.TrimSpace(next.Promote); token != "" {
		a.Promote = token
	}
	if token := strings.TrimSpace(next.Rollback); token != "" {
		a.Rollback = token
	}
}

// ApplyTo records only the handled tokens on the freshly read status. It does
// not consult spec, which may contain newer requests that remain pending.
func (a RequestAcknowledgements) ApplyTo(status *openbaov1alpha1.OpenBaoClusterStatus) {
	if token := strings.TrimSpace(a.Retry); token != "" {
		MarkRetryRequestHandled(status, token)
	}
	if token := strings.TrimSpace(a.Promote); token != "" {
		MarkPromoteRequestHandled(status, token)
	}
	if token := strings.TrimSpace(a.Rollback); token != "" {
		MarkRollbackRequestHandled(status, token)
	}
}
