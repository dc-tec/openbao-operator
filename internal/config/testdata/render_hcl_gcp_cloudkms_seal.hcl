ui               = true
cluster_name     = "gcp-seal"
api_addr         = "https://$${HOSTNAME}.gcp-seal.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.gcp-seal.default.svc:8201"
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
seal "gcpckms" {
  project     = "my-project"
  region      = "us-central1"
  key_ring    = "my-keyring"
  crypto_key  = "my-cryptokey"
  credentials = "/etc/gcp/credentials.json"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=gcp-seal\""
    leader_tls_servername   = "openbao-cluster-gcp-seal.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
