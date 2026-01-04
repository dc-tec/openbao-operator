ui               = true
cluster_name     = "gitops-contract"
api_addr         = "https://$${HOSTNAME}.gitops-contract.default.svc:8200"
cluster_addr     = "https://$${HOSTNAME}.gitops-contract.default.svc:8201"
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
    auto_join               = "provider=k8s namespace=default label_selector=\"openbao.org/cluster=gitops-contract\""
    leader_tls_servername   = "openbao-cluster-gitops-contract.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
service_registration "kubernetes" {
}
audit "file" "stdout" {
  description = "File audit device"
  options = {
    file_path = "stdout"
    mode      = "0600"
  }
}
audit "socket" "custom-socket" {
  description = "Socket audit device (raw options)"
  options = {
    address = "127.0.0.1:9000"
    timeout = "42"
  }
}
plugin "secret" "example" {
  image       = "ghcr.io/example/openbao-plugin"
  version     = "1.2.3"
  binary_name = "openbao-plugin"
  sha256sum   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  args        = ["--flag"]
  env         = ["KEY=value"]
}
telemetry {
  usage_gauge_period        = "10m"
  maximum_gauge_cardinality = 2000
  disable_hostname          = true
  enable_hostname_label     = true
  dogstatsd_addr            = "127.0.0.1:8125"
  dogstatsd_tags            = ["env:test", "cluster:gitops-contract"]
}
