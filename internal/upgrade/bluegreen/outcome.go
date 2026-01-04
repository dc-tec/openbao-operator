package bluegreen

import (
	"fmt"
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

type phaseOutcomeKind string

const (
	phaseOutcomeAdvance      phaseOutcomeKind = "advance"
	phaseOutcomeRequeueAfter phaseOutcomeKind = "requeueAfter"
	phaseOutcomeHold         phaseOutcomeKind = "hold"
	phaseOutcomeRollback     phaseOutcomeKind = "rollback"
	phaseOutcomeAbort        phaseOutcomeKind = "abort"
	phaseOutcomeDone         phaseOutcomeKind = "done"
)

type phaseOutcome struct {
	kind      phaseOutcomeKind
	nextPhase openbaov1alpha1.BlueGreenPhase
	after     time.Duration
	reason    string
}

func (o phaseOutcome) validate() error {
	switch o.kind {
	case phaseOutcomeAdvance:
		if o.nextPhase == "" {
			return fmt.Errorf("advance outcome requires nextPhase")
		}
	case phaseOutcomeRequeueAfter:
		if o.after <= 0 {
			return fmt.Errorf("requeueAfter outcome requires after > 0")
		}
	case phaseOutcomeRollback, phaseOutcomeAbort:
		if o.reason == "" {
			return fmt.Errorf("%s outcome requires reason", o.kind)
		}
	case phaseOutcomeHold, phaseOutcomeDone:
		// No fields required.
	default:
		return fmt.Errorf("unknown outcome kind: %q", o.kind)
	}
	return nil
}

func advance(to openbaov1alpha1.BlueGreenPhase) phaseOutcome {
	return phaseOutcome{kind: phaseOutcomeAdvance, nextPhase: to}
}

func requeueAfterOutcome(after time.Duration) phaseOutcome {
	return phaseOutcome{kind: phaseOutcomeRequeueAfter, after: after}
}

func hold() phaseOutcome {
	return phaseOutcome{kind: phaseOutcomeHold}
}

func rollback(reason string) phaseOutcome {
	return phaseOutcome{kind: phaseOutcomeRollback, reason: reason}
}
