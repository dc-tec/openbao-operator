ui               = true
cluster_name     = "oci-seal"
api_addr         = "https://$${HOSTNAME}.oci-seal.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.oci-seal.default.svc:8201"
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
seal "ocikms" {
  key_id              = "ocid1.key.oc1..example"
  crypto_endpoint     = "https://kms.us-ashburn-1.oraclecloud.com"
  management_endpoint = "https://kms.us-ashburn-1.oraclecloud.com"
  auth_type           = "instance_principal"
  compartment_id      = "ocid1.compartment.oc1..example"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=oci-seal\""
    leader_tls_servername   = "openbao-cluster-oci-seal.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
