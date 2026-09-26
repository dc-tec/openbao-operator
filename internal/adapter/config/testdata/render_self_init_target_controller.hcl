initialize "operator-bootstrap" {
  request "enable-jwt-auth" {
    operation = "update"
    path      = "sys/auth/jwt-operator"
    data {
      type        = "jwt"
      description = "Auth method for OpenBao Operator"
    }
  }
  request "config-jwt-auth" {
    operation = "update"
    path      = "auth/jwt-operator/config"
    data {
      bound_issuer           = "https://issuer"
      jwt_validation_pubkeys = ["test-public-key"]
    }
  }
  request "create-operator-policy" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator"
    data {
      policy = "path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/configuration\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/remove-peer\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/autopilot/configuration\" { capabilities = [\"read\", \"update\"] }\npath \"sys/storage/raft/autopilot/state\" { capabilities = [\"read\"] }"
    }
  }
  request "create-policy-approval" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator-policy-approval"
    data {
      policy = "path \"sys/policies/acl/openbao-operator\" {\n  capabilities = [\"read\", \"update\"]\n  allowed_parameters = {\n    policy = [\"path \\\"sys/health\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/step-down\\\" { capabilities = [\\\"sudo\\\", \\\"update\\\"] }\\npath \\\"sys/storage/raft/configuration\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/storage/raft/remove-peer\\\" { capabilities = [\\\"update\\\"] }\\npath \\\"sys/storage/raft/autopilot/configuration\\\" { capabilities = [\\\"read\\\", \\\"update\\\"] }\\npath \\\"sys/storage/raft/autopilot/state\\\" { capabilities = [\\\"read\\\"] }\"]\n  }\n}\npath \"sys/policies/acl/openbao-operator-upgrade\" {\n  capabilities = [\"read\", \"update\"]\n  allowed_parameters = {\n    policy = [\"path \\\"sys/health\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/step-down\\\" { capabilities = [\\\"sudo\\\", \\\"update\\\"] }\\npath \\\"sys/storage/raft/snapshot\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/storage/raft/autopilot/state\\\" { capabilities = [\\\"read\\\"] }\", \"path \\\"sys/health\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/step-down\\\" { capabilities = [\\\"sudo\\\", \\\"update\\\"] }\\npath \\\"sys/storage/raft/snapshot\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/storage/raft/autopilot/state\\\" { capabilities = [\\\"read\\\"] }\\npath \\\"sys/storage/raft/join\\\" { capabilities = [\\\"update\\\"] }\\npath \\\"sys/storage/raft/configuration\\\" { capabilities = [\\\"read\\\", \\\"update\\\"] }\\npath \\\"sys/storage/raft/remove-peer\\\" { capabilities = [\\\"update\\\"] }\\npath \\\"sys/storage/raft/promote\\\" { capabilities = [\\\"update\\\"] }\\npath \\\"sys/storage/raft/demote\\\" { capabilities = [\\\"update\\\"] }\"]\n  }\n}\npath \"sys/policies/acl/openbao-operator-restore\" {\n  capabilities = [\"read\", \"update\"]\n  allowed_parameters = {\n    policy = [\"path \\\"sys/storage/raft/snapshot\\\" { capabilities = [\\\"update\\\"] }\\npath \\\"sys/storage/raft/snapshot-force\\\" { capabilities = [\\\"update\\\"] }\"]\n  }\n}\npath \"sys/policies/acl/openbao-operator-backup\" {\n  capabilities = [\"read\", \"update\"]\n  allowed_parameters = {\n    policy = [\"path \\\"sys/storage/raft/snapshot\\\" { capabilities = [\\\"read\\\"] }\"]\n  }\n}\n"
    }
  }
  request "create-operator-role" {
    operation = "update"
    path      = "auth/jwt-operator/role/openbao-operator"
    data {
      role_type               = "jwt"
      user_claim              = "sub"
      bound_audiences         = ["urn:openbao:controller:cluster-uid"]
      bound_subject           = "system:serviceaccount:operator:controller"
      token_policies          = ["openbao-operator", "openbao-operator-policy-approval"]
      policies                = ["openbao-operator", "openbao-operator-policy-approval"]
      ttl                     = "1h"
      token_ttl               = "1h"
      token_max_ttl           = "1h"
      token_no_default_policy = true
      clock_skew_leeway       = "30s"
      expiration_leeway       = "30s"
      not_before_leeway       = "30s"
    }
  }
  request "create-backup-policy" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator-backup"
    data {
      policy = "path \"sys/storage/raft/snapshot\" { capabilities = [\"read\"] }"
    }
  }
  request "create-backup-jwt-role" {
    operation = "update"
    path      = "auth/jwt-operator/role/openbao-operator-backup"
    data {
      role_type               = "jwt"
      user_claim              = "sub"
      bound_audiences         = ["legacy-audience"]
      bound_subject           = "system:serviceaccount:tenant:bao-backup-serviceaccount"
      token_policies          = ["openbao-operator-backup"]
      policies                = ["openbao-operator-backup"]
      ttl                     = "1h"
      token_ttl               = "1h"
      token_max_ttl           = "1h"
      token_no_default_policy = true
      clock_skew_leeway       = "30s"
      expiration_leeway       = "30s"
      not_before_leeway       = "30s"
    }
  }
  request "create-upgrade-policy" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator-upgrade"
    data {
      policy = "path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/snapshot\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/autopilot/state\" { capabilities = [\"read\"] }"
    }
  }
  request "create-upgrade-jwt-role" {
    operation = "update"
    path      = "auth/jwt-operator/role/openbao-operator-upgrade"
    data {
      role_type               = "jwt"
      user_claim              = "sub"
      bound_audiences         = ["legacy-audience"]
      bound_subject           = "system:serviceaccount:tenant:bao-upgrade-serviceaccount"
      token_policies          = ["openbao-operator-upgrade"]
      policies                = ["openbao-operator-upgrade"]
      ttl                     = "1h"
      token_ttl               = "1h"
      token_max_ttl           = "1h"
      token_no_default_policy = true
      clock_skew_leeway       = "30s"
      expiration_leeway       = "30s"
      not_before_leeway       = "30s"
    }
  }
  request "create-restore-policy" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator-restore"
    data {
      policy = "path \"sys/storage/raft/snapshot\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/snapshot-force\" { capabilities = [\"update\"] }"
    }
  }
  request "create-restore-jwt-role" {
    operation = "update"
    path      = "auth/jwt-operator/role/openbao-operator-restore"
    data {
      role_type               = "jwt"
      user_claim              = "sub"
      bound_audiences         = ["legacy-audience"]
      bound_subject           = "system:serviceaccount:tenant:bao-restore-serviceaccount"
      token_policies          = ["openbao-operator-restore"]
      policies                = ["openbao-operator-restore"]
      ttl                     = "1h"
      token_ttl               = "1h"
      token_max_ttl           = "1h"
      token_no_default_policy = true
      clock_skew_leeway       = "30s"
      expiration_leeway       = "30s"
      not_before_leeway       = "30s"
    }
  }
}
initialize "configure-autopilot" {
  request "configure-autopilot-request" {
    operation = "update"
    path      = "sys/storage/raft/autopilot/configuration"
    data {
      cleanup_dead_servers               = true
      dead_server_last_contact_threshold = "24h"
      min_quorum                         = "3"
      server_stabilization_time          = "10s"
    }
  }
}
