package openbao

// RaftServer represents a server in the Raft configuration.
type RaftServer struct {
	NodeID          string `json:"node_id"`
	Address         string `json:"address"`
	Leader          bool   `json:"leader,omitempty"`
	ProtocolVersion string `json:"protocol_version,omitempty"`
	Voter           bool   `json:"voter,omitempty"`
	LastIndex       uint64 `json:"last_index,omitempty"`
	LastTerm        uint64 `json:"last_term,omitempty"`
}

// RaftConfiguration represents the current Raft cluster configuration.
type RaftConfiguration struct {
	Servers []RaftServer `json:"servers"`
	Index   uint64       `json:"index"`
}

// RaftConfigurationResponse represents the response from the raft configuration API.
type RaftConfigurationResponse struct {
	Config RaftConfiguration `json:"config"`
}
