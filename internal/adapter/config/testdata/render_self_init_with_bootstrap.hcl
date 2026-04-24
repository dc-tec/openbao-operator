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
      bound_issuer           = "https://kubernetes.default.svc"
      jwt_validation_pubkeys = ["-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"]
    }
  }
  request "create-operator-policy" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator"
    data {
      policy = "path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/configuration\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/remove-peer\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/autopilot/configuration\" { capabilities = [\"read\", \"update\"] }\npath \"sys/storage/raft/autopilot/state\" { capabilities = [\"read\"] }"
    }
  }
  request "create-operator-role" {
    operation = "update"
    path      = "auth/jwt-operator/role/openbao-operator"
    data {
      role_type               = "jwt"
      user_claim              = "sub"
      bound_audiences         = ["openbao-internal"]
      bound_subject           = "system:serviceaccount:openbao-operator-system:openbao-operator-controller"
      token_policies          = ["openbao-operator"]
      policies                = ["openbao-operator"]
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
initialize "enable-stdout-audit" {
  request "enable-stdout-audit-request" {
    operation = "update"
    path      = "sys/audit/stdout"
    data {
      options = {
        file_path = "stdout"
      }
      type = "file"
    }
  }
}
