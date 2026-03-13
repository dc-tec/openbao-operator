ui               = true
cluster_name     = "pkcs11-seal"
api_addr         = "https://$${HOSTNAME}.pkcs11-seal.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.pkcs11-seal.default.svc:8201"
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
seal "pkcs11" {
  lib                         = "/usr/lib/libpkcs11.so"
  token_label                 = "openbao-token"
  key_label                   = "openbao-hsm-key"
  key_id                      = "01"
  mechanism                   = "0x0009"
  disable_software_encryption = "true"
  rsa_oaep_hash               = "sha256"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=pkcs11-seal\""
    leader_tls_servername   = "openbao-cluster-pkcs11-seal.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
