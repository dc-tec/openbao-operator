request "create-policy-approver" {
  operation = "update"
  path      = "sys/policies/acl/openbao-operator-policy-approver"
  data {
    policy = "path \"sys/policies/acl/openbao-operator-policy-approval\" {\n  capabilities = [\"read\", \"update\"]\n}\n"
  }
}
request "bind-policy-approver" {
  operation = "update"
  path      = "auth/jwt-operator/role/openbao-operator-policy-approver"
  data {
    role_type               = "jwt"
    user_claim              = "sub"
    bound_audiences         = ["openbao-policy-approval:bao:example"]
    bound_subject           = "system:serviceaccount:openbao-admin:example-policy-approver"
    token_policies          = ["openbao-operator-policy-approver"]
    ttl                     = "5m"
    token_ttl               = "5m"
    token_max_ttl           = "5m"
    token_explicit_max_ttl  = "5m"
    token_no_default_policy = true
    clock_skew_leeway       = "30s"
    expiration_leeway       = "30s"
    not_before_leeway       = "30s"
  }
}
