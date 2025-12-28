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
      bound_claims = {
        "kubernetes.io/namespace"           = "openbao-operator-system"
        "kubernetes.io/serviceaccount/name" = "openbao-operator-controller"
      }
      token_policies = ["openbao-operator"]
      policies       = ["openbao-operator"]
      ttl            = "1h"
    }
  }
  request "create-backup-policy" {
    operation = "update"
    path      = "sys/policies/acl/backup"
    data {
      policy = "path \"sys/storage/raft/snapshot\" { capabilities = [\"read\"] }"
    }
  }
  request "create-backup-jwt-role" {
    operation = "update"
    path      = "auth/jwt/role/backup"
    data {
      role_type       = "jwt"
      user_claim      = "sub"
      bound_audiences = ["openbao-internal"]
      bound_claims = {
        "kubernetes.io/namespace"           = "default"
        "kubernetes.io/serviceaccount/name" = "hardened-cluster-backup-serviceaccount"
      }
      token_policies = ["backup"]
      policies       = ["backup"]
      ttl            = "1h"
    }
  }
  request "create-upgrade-policy" {
    operation = "update"
    path      = "sys/policies/acl/upgrade"
    data {
      policy = "path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/snapshot\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/autopilot/state\" { capabilities = [\"read\"] }"
    }
  }
  request "create-upgrade-jwt-role" {
    operation = "update"
    path      = "auth/jwt/role/upgrade"
    data {
      role_type       = "jwt"
      user_claim      = "sub"
      bound_audiences = ["openbao-internal"]
      bound_claims = {
        "kubernetes.io/namespace"           = "default"
        "kubernetes.io/serviceaccount/name" = "hardened-cluster-upgrade-serviceaccount"
      }
      token_policies = ["upgrade"]
      policies       = ["upgrade"]
      ttl            = "1h"
    }
  }
}
