path "sys/policies/acl/openbao-operator" {
  capabilities = ["read", "update"]
  allowed_parameters = {
    policy = ["path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/configuration\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/remove-peer\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/autopilot/configuration\" { capabilities = [\"read\", \"update\"] }\npath \"sys/storage/raft/autopilot/state\" { capabilities = [\"read\"] }"]
  }
}
path "sys/policies/acl/openbao-operator-upgrade" {
  capabilities = ["read", "update"]
  allowed_parameters = {
    policy = ["path \"sys/health\" { capabilities = [\"read\"] }\npath \"sys/step-down\" { capabilities = [\"sudo\", \"update\"] }\npath \"sys/storage/raft/snapshot\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/autopilot/state\" { capabilities = [\"read\"] }\npath \"sys/storage/raft/join\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/configuration\" { capabilities = [\"read\", \"update\"] }\npath \"sys/storage/raft/remove-peer\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/promote\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/demote\" { capabilities = [\"update\"] }"]
  }
}
path "sys/policies/acl/openbao-operator-restore" {
  capabilities = ["read", "update"]
  allowed_parameters = {
    policy = ["path \"sys/storage/raft/snapshot\" { capabilities = [\"update\"] }\npath \"sys/storage/raft/snapshot-force\" { capabilities = [\"update\"] }"]
  }
}
