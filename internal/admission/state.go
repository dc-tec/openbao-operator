package admission

import "sync/atomic"

var admissionDependenciesReady atomic.Bool

// AdmissionDependenciesReady reports whether required admission dependencies were last observed as ready.
func AdmissionDependenciesReady() bool {
	return admissionDependenciesReady.Load()
}

func setAdmissionDependenciesReady(ready bool) {
	admissionDependenciesReady.Store(ready)
}
