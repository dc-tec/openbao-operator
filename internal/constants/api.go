package constants

// OpenBao API paths used by the operator and helper binaries.
const (
	APIPathSysHealth    = "/v1/sys/health"
	APIPathSysInit      = "/v1/sys/init"
	APIPathSysStepDown  = "/v1/sys/step-down"
	APIPathRaftSnapshot = "/v1/sys/storage/raft/snapshot"
	APIPathAuthJWTLogin = "/v1/auth/jwt/login"
)
