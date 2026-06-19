initialize "initial-recovery-keys" {
  request "create-initial-recovery-keys" {
    operation = "update"
    path      = "sys/rotate/recovery/init"
    data {
      secret_shares    = 3
      secret_threshold = 2
      backup           = true
      pgp_keys         = ["pgp-key-one", "pgp-key-two", "pgp-key-three"]
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
