ui               = true
cluster_name     = "awskms-seal"
api_addr         = "https://$${HOSTNAME}.awskms-seal.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.awskms-seal.default.svc:8201"
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
seal "awskms" {
  region     = "us-east-1"
  kms_key_id = "alias/my-key"
  endpoint   = "https://kms.us-east-1.amazonaws.com"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=awskms-seal\""
    leader_tls_servername   = "openbao-cluster-awskms-seal.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
