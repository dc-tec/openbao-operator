package openbao

const (
	apiPathSysHealth                = "/v1/sys/health"
	apiPathSysInit                  = "/v1/sys/init"
	apiPathSysLeader                = "/v1/sys/leader"
	apiPathSysStepDown              = "/v1/sys/step-down"
	apiPathRaftSnapshot             = "/v1/sys/storage/raft/snapshot"
	apiPathRaftJoin                 = "/v1/sys/storage/raft/join"
	apiPathRaftConfiguration        = "/v1/sys/storage/raft/configuration"
	apiPathRaftRemovePeer           = "/v1/sys/storage/raft/remove-peer"
	apiPathRaftPromotePeer          = "/v1/sys/storage/raft/promote"
	apiPathRaftDemotePeer           = "/v1/sys/storage/raft/demote"
	apiPathRaftSnapshotForceRestore = "/v1/sys/storage/raft/snapshot-force"
	apiPathRaftAutopilotConfig      = "/v1/sys/storage/raft/autopilot/configuration"
	apiPathRaftAutopilotState       = "/v1/sys/storage/raft/autopilot/state"
	apiPathRaftUpdateConfig         = "/v1/sys/storage/raft/configuration"
	apiPathAuthJWTLogin             = "/v1/auth/jwt-operator/login"
)
