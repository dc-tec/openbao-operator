package v1alpha1

// ControllerJWTMode selects shared or target-specific controller credentials.
// +kubebuilder:validation:Enum=Shared;Target
type ControllerJWTMode string

const (
	// ControllerJWTModeShared uses the installation's projected ServiceAccount JWT.
	ControllerJWTModeShared ControllerJWTMode = "Shared"
	// ControllerJWTModeTarget requests a JWT for this OpenBaoCluster's UID.
	ControllerJWTModeTarget ControllerJWTMode = "Target"
)
