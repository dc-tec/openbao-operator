package openbao

import "errors"

var (
	// ErrAutopilotNotAvailable indicates the Raft Autopilot state endpoint is not available.
	// This can happen when Autopilot is disabled or not supported by the running OpenBao build.
	ErrAutopilotNotAvailable = errors.New("raft autopilot state endpoint not available")
)
