ui               = true
cluster_name     = "structured-config"
api_addr         = "https://$${HOSTNAME}.structured-config.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.structured-config.default.svc:8201"
plugin_directory = "/openbao/plugins"
listener "tcp" {
  address              = "[::]:8200"
  cluster_address      = "[::]:8201"
  tls_disable          = 0
  max_request_duration = "90s"
  tls_cert_file        = "/etc/bao/tls/tls.crt"
  tls_key_file         = "/etc/bao/tls/tls.key"
  tls_client_ca_file   = "/etc/bao/tls/ca.crt"
}
seal "static" {
  current_key    = "file:///etc/bao/unseal/key"
  current_key_id = "operator-generated-v1"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=structured-config\""
    leader_tls_servername   = "openbao-cluster-structured-config.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
log_level                = "info"
log_format               = "json"
log_file                 = "/var/log/openbao/openbao.log"
log_rotate_duration      = "24h"
log_rotate_bytes         = 10485760
log_rotate_max_files     = 7
pid_file                 = "/var/run/openbao/openbao.pid"
plugin_file_uid          = 1000
plugin_file_permissions  = "0755"
plugin_auto_download     = true
plugin_auto_register     = false
plugin_download_behavior = "direct"
default_lease_ttl        = "720h"
max_lease_ttl            = "8760h"
