package openbaocluster

import (
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// Reason constants for OpenBaoCluster conditions.
const (
	ReasonOIDCBootstrapConfigurationInvalid = constants.ReasonOIDCBootstrapConfigurationInvalid
	ReasonUnsafeAdmissionDisabled           = "UnsafeAdmissionDisabled"
	ReasonDevelopmentProfile                = "DevelopmentProfile"
	ReasonAmbientUnsealIdentity             = "AmbientUnsealIdentity"
	ReasonProfileNotSet                     = "ProfileNotSet"
	ReasonRootTokenStored                   = "RootTokenStored"
	ReasonStaticUnsealInUse                 = "StaticUnsealInUse"

	ReasonImageVersionMismatch = constants.ReasonImageVersionMismatch

	reasonUnknown = constants.ReasonUnknown

	controllerNameWorkload = "openbaocluster-workload"
	controllerNameAdminOps = "openbaocluster-adminops"
	controllerNameStatus   = "openbaocluster-status"

	annotationLastDevelopmentWarning     = "openbao.org/last-development-warning"
	annotationLastAmbientUnsealNote      = "openbao.org/last-ambient-unseal-identity-note"
	annotationLastProfileNotSetWarning   = "openbao.org/last-profile-not-set-warning"
	annotationLastRootTokenWarning       = "openbao.org/last-root-token-warning"
	annotationLastStaticUnsealWarning    = "openbao.org/last-static-unseal-warning"
	annotationLastUnsafeAdmissionWarning = "openbao.org/last-unsafe-admission-warning"
)
const securityWarningInterval = time.Hour
