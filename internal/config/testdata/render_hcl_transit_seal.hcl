ui               = true
cluster_name     = "transit-seal"
api_addr         = "https://$${HOSTNAME}.transit-seal.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.transit-seal.default.svc:8201"
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
seal "transit" {
  address         = "https://openbao:8200"
  key_name        = "transit-key"
  mount_path      = "transit/"
  namespace       = "ns1/"
  tls_ca_cert     = "/etc/openbao/ca_cert.pem"
  tls_skip_verify = "false"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=transit-seal\""
    leader_tls_servername   = "openbao-cluster-transit-seal.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
