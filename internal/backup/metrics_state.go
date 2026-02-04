package backup

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

var backupJobMetricsSeen sync.Map // key: string -> struct{}

func backupJobMetricsSeenKey(namespace, name string, jobUID types.UID, outcome string) string {
	return namespace + "/" + name + "/" + string(jobUID) + "/" + outcome
}

func markBackupJobMetricsSeen(namespace, name string, jobUID types.UID, outcome string) bool {
	_, loaded := backupJobMetricsSeen.LoadOrStore(backupJobMetricsSeenKey(namespace, name, jobUID, outcome), struct{}{})
	return !loaded
}

