package constants

// OpenBao API paths used by the operator and helper binaries.
const (
	APIPathSysHealth                = "/v1/sys/health"
	APIPathSysInit                  = "/v1/sys/init"
	APIPathSysLeader                = "/v1/sys/leader"
	APIPathSysStepDown              = "/v1/sys/step-down"
	APIPathRaftSnapshot             = "/v1/sys/storage/raft/snapshot"
	APIPathRaftJoin                 = "/v1/sys/storage/raft/join"
	APIPathRaftConfiguration        = "/v1/sys/storage/raft/configuration"
	APIPathRaftRemovePeer           = "/v1/sys/storage/raft/remove-peer"
	APIPathRaftPromotePeer          = "/v1/sys/storage/raft/promote"
	APIPathRaftDemotePeer           = "/v1/sys/storage/raft/demote"
	APIPathRaftSnapshotRestore      = "/v1/sys/storage/raft/snapshot/restore"
	APIPathRaftSnapshotForceRestore = "/v1/sys/storage/raft/snapshot-force"
	APIPathRaftAutopilotConfig      = "/v1/sys/storage/raft/autopilot/configuration"
	APIPathRaftAutopilotState       = "/v1/sys/storage/raft/autopilot/state"
	APIPathRaftUpdateConfig         = "/v1/sys/storage/raft/configuration"
	APIPathAuthJWTLogin             = "/v1/auth/jwt-operator/login"
)
