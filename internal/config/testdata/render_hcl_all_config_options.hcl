ui               = true
cluster_name     = "full-config"
api_addr         = "https://$${HOSTNAME}.full-config.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.full-config.default.svc:8201"
plugin_directory = "/openbao/plugins"
listener "tcp" {
  address              = "0.0.0.0:8200"
  cluster_address      = "0.0.0.0:8201"
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
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=full-config\""
    leader_tls_servername   = "openbao-cluster-full-config.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
log_level                           = "warn"
default_lease_ttl                   = "3600h"
max_lease_ttl                       = "7200h"
cache_size                          = 134217728
disable_cache                       = false
detect_deadlocks                    = "true"
raw_storage_endpoint                = false
introspection_endpoint              = true
imprecise_lease_role_tracking       = true
unsafe_allow_api_audit_creation     = false
allow_audit_log_prefixing           = true
enable_response_header_hostname     = true
enable_response_header_raft_node_id = false
