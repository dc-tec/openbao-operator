package bluegreen

import "github.com/dc-tec/openbao-operator/internal/upgrade"

// ExecutorAction selects which Blue/Green upgrade operation the upgrade executor performs.
type ExecutorAction = upgrade.ExecutorAction

const (
	ActionJoinGreenNonVoters          ExecutorAction = upgrade.ExecutorActionBlueGreenJoinGreenNonVoters
	ActionWaitGreenSynced             ExecutorAction = upgrade.ExecutorActionBlueGreenWaitGreenSynced
	ActionPromoteGreenVoters          ExecutorAction = upgrade.ExecutorActionBlueGreenPromoteGreenVoters
	ActionDemoteBlueNonVotersStepDown ExecutorAction = upgrade.ExecutorActionBlueGreenDemoteBlueNonVotersStepDown
	ActionRemoveBluePeers             ExecutorAction = upgrade.ExecutorActionBlueGreenRemoveBluePeers

	// ActionRepairConsensus repairs Raft consensus during rollback by ensuring
	// Blue nodes are voters and Green nodes are non-voters in a single pass.
	ActionRepairConsensus ExecutorAction = upgrade.ExecutorActionBlueGreenRepairConsensus
)
