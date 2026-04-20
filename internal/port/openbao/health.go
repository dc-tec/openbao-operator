package openbao

// HealthStatus represents the response from GET /v1/sys/health.
// The health endpoint returns different status codes based on cluster state:
// - 200: initialized, unsealed, and active
// - 429: unsealed and standby
// - 472: data recovery mode replication secondary and target sealed
// - 473: performance standby
// - 501: not initialized
// - 503: sealed
type HealthStatus struct {
	// Initialized indicates whether OpenBao has been initialized.
	Initialized bool `json:"initialized"`
	// Sealed indicates whether OpenBao is sealed.
	Sealed bool `json:"sealed"`
	// Standby indicates whether this node is a standby (not the leader).
	Standby bool `json:"standby"`
	// PerformanceStandby indicates if this is a performance standby node.
	PerformanceStandby bool `json:"performance_standby"`
	// ReplicationPerformanceMode is the replication mode.
	ReplicationPerformanceMode string `json:"replication_performance_mode,omitempty"`
	// ReplicationDRMode is the DR replication mode.
	ReplicationDRMode string `json:"replication_dr_mode,omitempty"`
	// ServerTimeUTC is the server time in UTC.
	ServerTimeUTC int64 `json:"server_time_utc,omitempty"`
	// Version is the OpenBao version.
	Version string `json:"version,omitempty"`
	// ClusterName is the name of the Raft cluster.
	ClusterName string `json:"cluster_name,omitempty"`
	// LeaderAddress is the address of the leader node.
	LeaderAddress string `json:"leader_address,omitempty"`
	// ClusterID is the unique identifier for the cluster.
	ClusterID string `json:"cluster_id,omitempty"`
}
