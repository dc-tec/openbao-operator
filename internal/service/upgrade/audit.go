package upgrade

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

// UpgradeStartedAuditFields builds the common audit-event fields for the start
// of an upgrade attempt.
func UpgradeStartedAuditFields(
	cluster *openbaov1alpha1.OpenBaoCluster,
	strategy string,
	fromVersion string,
	toVersion string,
) map[string]string {
	fields := upgradeAuditFields(cluster, strategy)
	fields["from_version"] = fromVersion
	fields["to_version"] = toVersion
	return fields
}

// UpgradeCompletedAuditFields builds the common audit-event fields for the
// successful completion of an upgrade attempt.
func UpgradeCompletedAuditFields(cluster *openbaov1alpha1.OpenBaoCluster, strategy string, version string) map[string]string {
	fields := upgradeAuditFields(cluster, strategy)
	fields["version"] = version
	return fields
}

func upgradeAuditFields(cluster *openbaov1alpha1.OpenBaoCluster, strategy string) map[string]string {
	fields := map[string]string{
		"strategy": strategy,
	}
	if cluster == nil {
		return fields
	}

	fields["cluster_namespace"] = cluster.Namespace
	fields["cluster_name"] = cluster.Name
	return fields
}
