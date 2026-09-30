ui           = true
cluster_name = "pq-tls"
api_addr     = "https://$${HOSTNAME}.pq-tls.default.svc:8200"
cluster_addr = "https://$${HOSTNAME}.pq-tls.default.svc:8201"
listener "tcp" {
  address                      = "[::]:8200"
  cluster_address              = "[::]:8201"
  tls_disable                  = 0
  max_request_duration         = "90s"
  tls_min_version              = "tls13"
  tls_max_version              = "tls13"
  tls_key_exchange_preferences = ["X25519MLKEM768", "SecP256r1MLKEM768", "SecP384r1MLKEM1024"]
  tls_cert_file                = "/etc/bao/tls/tls.crt"
  tls_key_file                 = "/etc/bao/tls/tls.key"
  tls_client_ca_file           = "/etc/bao/tls/ca.crt"
  tls_auto_reload              = true
  tls_auto_reload_interval     = "10s"
  telemetry {
    disallow_metrics = true
  }
}
listener "tcp" {
  address                      = "[::]:8202"
  tls_disable                  = 0
  max_request_duration         = "90s"
  tls_min_version              = "tls13"
  tls_max_version              = "tls13"
  tls_key_exchange_preferences = ["X25519MLKEM768", "SecP256r1MLKEM768", "SecP384r1MLKEM1024"]
  tls_cert_file                = "/etc/bao/tls/tls.crt"
  tls_key_file                 = "/etc/bao/tls/tls.key"
  tls_client_ca_file           = "/etc/bao/tls/ca.crt"
  tls_auto_reload              = true
  tls_auto_reload_interval     = "10s"
  telemetry {
    unauthenticated_metrics_access = true
    metrics_only                   = true
  }
}
seal "static" {
  current_key    = "file:///etc/bao/unseal/key"
  current_key_id = "operator-generated-v1"
}
storage "raft" {
  path    = "/bao/data"
  node_id = "$${HOSTNAME}"
  retry_join {
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=pq-tls\""
    leader_tls_servername   = "openbao-cluster-pq-tls.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
telemetry {
  disable_hostname          = true
  prometheus_retention_time = "30s"
}
