package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func buildUpgradeExecutorJobAnnotations(action ExecutorAction, runID string) map[string]string {
	annotations := map[string]string{
		"openbao.org/upgrade-action": string(action),
	}
	if strings.TrimSpace(runID) != "" {
		annotations["openbao.org/upgrade-run-id"] = runID
	}
	return annotations
}

func upgradeExecutorJobName(clusterName string, action ExecutorAction, runID string, blueRevision, greenRevision string) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s", clusterName, action, runID, blueRevision, greenRevision)
	sum := sha256.Sum256([]byte(payload))
	suffix := hex.EncodeToString(sum[:])[:10]

	base := fmt.Sprintf("%s%s-%s", upgradeJobNamePrefix, clusterName, string(action))
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, "_", "-")

	maxBaseLen := 63 - 1 - len(suffix)
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
		base = strings.TrimRight(base, "-")
	}

	return fmt.Sprintf("%s-%s", base, suffix)
}
