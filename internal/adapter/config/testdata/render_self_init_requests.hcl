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
