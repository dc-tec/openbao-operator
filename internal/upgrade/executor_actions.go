package upgrade

// ExecutorAction selects which upgrade operation the upgrade executor performs.
type ExecutorAction string

const (
	ExecutorActionBlueGreenJoinGreenNonVoters          ExecutorAction = "bluegreen-join-green-nonvoters"
	ExecutorActionBlueGreenWaitGreenSynced             ExecutorAction = "bluegreen-wait-green-synced"
	ExecutorActionBlueGreenPromoteGreenVoters          ExecutorAction = "bluegreen-promote-green-voters"
	ExecutorActionBlueGreenDemoteBlueNonVotersStepDown ExecutorAction = "bluegreen-demote-blue-nonvoters-stepdown"
	ExecutorActionBlueGreenRemoveBluePeers             ExecutorAction = "bluegreen-remove-blue-peers"

	ExecutorActionRollingStepDownLeader ExecutorAction = "rolling-stepdown-leader"
)
