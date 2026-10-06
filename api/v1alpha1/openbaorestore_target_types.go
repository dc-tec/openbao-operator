package v1alpha1

// RestoreAdministratorDisposition records the administrator's recovery decision.
type RestoreAdministratorDisposition string

const (
	RestoreAdministratorResume  RestoreAdministratorDisposition = "Resume"
	RestoreAdministratorAbandon RestoreAdministratorDisposition = "Abandon"
)
