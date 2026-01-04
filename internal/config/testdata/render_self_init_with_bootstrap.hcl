initialize "operator-bootstrap" {
  request "enable-jwt-auth" {
    operation = "update"
    path      = "sys/auth/jwt"
    data {
      type        = "jwt"
      description = "Auth method for OpenBao Operator"
    }
  }
  request "config-jwt-auth" {
    operation = "update"
    path      = "auth/jwt/config"
    data {
      bound_issuer           = "https://kubernetes.default.svc"
      jwt_validation_pubkeys = ["-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"]
    }
  }
  request "create-operator-policy" {
    operation = "update"
    path      = "sys/policies/acl/openbao-operator"
    data {
      policy = "path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/snapshot\" { capabilities = [\"read\"] }"
    }
  }
  request "create-operator-role" {
    operation = "update"
    path      = "auth/jwt/role/openbao-operator"
    data {
      role_type       = "jwt"
      user_claim      = "sub"
      bound_audiences = ["openbao-internal"]
      bound_subject   = "system:serviceaccount:openbao-operator-system:openbao-operator-controller"
      token_policies  = ["openbao-operator"]
      policies        = ["openbao-operator"]
      ttl             = "1h"
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
