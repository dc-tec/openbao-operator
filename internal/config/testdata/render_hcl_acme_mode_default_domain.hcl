ui               = true
cluster_name     = "acme-cluster"
api_addr         = "https://$${HOSTNAME}.acme-cluster.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.acme-cluster.default.svc:8201"
plugin_directory = "/openbao/plugins"
listener "tcp" {
  address                         = "[::]:8200"
  cluster_address                 = "[::]:8201"
  tls_disable                     = 0
  max_request_duration            = "90s"
  tls_acme_ca_directory           = "https://acme-v02.api.letsencrypt.org/directory"
  tls_acme_domains                = ["acme-cluster-acme.default.svc"]
  tls_acme_disable_http_challenge = true
}
seal "static" {
  current_key    = "file:///etc/bao/unseal/key"
  current_key_id = "operator-generated-v1"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join             = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=acme-cluster\""
    leader_tls_servername = "acme-cluster-acme.default.svc"
  }
}
service_registration "kubernetes" {
}
