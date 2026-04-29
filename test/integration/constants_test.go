//go:build integration
// +build integration

package integration

const (
	testAdmissionRegistrationGroup = "admissionregistration.k8s.io"
	testAdmissionRegistrationV1    = testAdmissionRegistrationGroup + "/v1"
	testKindVAP                    = "ValidatingAdmissionPolicy"
	testKindVAPBinding             = "ValidatingAdmissionPolicyBinding"

	testRBACGroup              = "rbac.authorization.k8s.io"
	testRBACV1                 = testRBACGroup + "/v1"
	testKindClusterRole        = "ClusterRole"
	testKindClusterRoleBinding = "ClusterRoleBinding"
	testKindRoleBinding        = "RoleBinding"
	testComponentProvisioner   = "provisioner"
	testControllerSAName       = "openbao-operator-controller"
	testDefaultOperatorNS      = "openbao-operator-system"
	testPrefixedControllerSA   = "demo-openbao-operator-controller"
	testPrefixedProvisionerSA  = "demo-openbao-operator-provisioner"
	testCustomOperatorNS       = "custom-operator"
	testQuotedCustomOperatorNS = "'custom-operator'"
	testQuotedPrefixedCtrlSA   = "'demo-openbao-operator-controller'"
	testQuotedPrefixedProvSA   = "'demo-openbao-operator-provisioner'"
	testSingleTenantTargetNS   = "openbao"
	testCustomTenantTargetNS   = "tenant-openbao"
	testBackupBucket           = "backups"
	testTrueString             = "true"
	testOpenBaoVersion244      = "2.4.4"
	testOpenBaoImage244        = "openbao/openbao:2.4.4"
	testOpenBaoVersion250      = "2.5.0"
	testOpenBaoImage250        = "openbao/openbao:2.5.0"
	testPreviousOpenBaoVersion = "2.4.3"
)
