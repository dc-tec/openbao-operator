package provisioner

import rbacv1 "k8s.io/api/rbac/v1"

type policyRulesBuilder struct {
	rules []rbacv1.PolicyRule
}

func newPolicyRulesBuilder() *policyRulesBuilder {
	return &policyRulesBuilder{}
}

func (b *policyRulesBuilder) Group(apiGroup string) *policyRuleBuilder {
	return &policyRuleBuilder{
		parent:   b,
		apiGroup: apiGroup,
	}
}

func (b *policyRulesBuilder) Rules() []rbacv1.PolicyRule {
	return b.rules
}

type policyRuleBuilder struct {
	parent    *policyRulesBuilder
	apiGroup  string
	resources []string
}

func (b *policyRuleBuilder) Resources(resources ...string) *policyRuleBuilder {
	b.resources = append(b.resources, resources...)
	return b
}

func (b *policyRuleBuilder) Verbs(verbs ...string) *policyRulesBuilder {
	resources := make([]string, len(b.resources))
	copy(resources, b.resources)

	verbsCopy := make([]string, len(verbs))
	copy(verbsCopy, verbs)

	b.parent.rules = append(b.parent.rules, rbacv1.PolicyRule{
		APIGroups: []string{b.apiGroup},
		Resources: resources,
		Verbs:     verbsCopy,
	})
	return b.parent
}
