package bluegreen

// ExecutorAction selects which Blue/Green upgrade operation the upgrade executor performs.
type ExecutorAction string

const (
	ActionJoinGreenNonVoters          ExecutorAction = "bluegreen-join-green-nonvoters"
	ActionWaitGreenSynced             ExecutorAction = "bluegreen-wait-green-synced"
	ActionPromoteGreenVoters          ExecutorAction = "bluegreen-promote-green-voters"
	ActionDemoteBlueNonVotersStepDown ExecutorAction = "bluegreen-demote-blue-nonvoters-stepdown"
	ActionRemoveBluePeers             ExecutorAction = "bluegreen-remove-blue-peers"

	// Rollback actions
	ActionPromoteBlueVoters    ExecutorAction = "bluegreen-promote-blue-voters"
	ActionDemoteGreenNonVoters ExecutorAction = "bluegreen-demote-green-nonvoters"
	ActionRemoveGreenPeers     ExecutorAction = "bluegreen-remove-green-peers"
)
