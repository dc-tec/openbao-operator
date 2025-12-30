package bluegreen

// ExecutorAction selects which Blue/Green upgrade operation the upgrade executor performs.
type ExecutorAction string

const (
	ActionJoinGreenNonVoters          ExecutorAction = "bluegreen-join-green-nonvoters"
	ActionWaitGreenSynced             ExecutorAction = "bluegreen-wait-green-synced"
	ActionPromoteGreenVoters          ExecutorAction = "bluegreen-promote-green-voters"
	ActionDemoteBlueNonVotersStepDown ExecutorAction = "bluegreen-demote-blue-nonvoters-stepdown"
	ActionRemoveBluePeers             ExecutorAction = "bluegreen-remove-blue-peers"

	// ActionRepairConsensus repairs Raft consensus during rollback by ensuring
	// Blue nodes are voters and Green nodes are non-voters in a single pass.
	ActionRepairConsensus ExecutorAction = "bluegreen-repair-consensus"
)
