package config

const (
	openBaoAPIPort     = 8200
	openBaoClusterPort = 8201

	openBaoPathData          = "/bao/data"
	openBaoPathTLSCACert     = "/etc/bao/tls/ca.crt"
	openBaoPathTLSServerCert = "/etc/bao/tls/tls.crt"
	openBaoPathTLSServerKey  = "/etc/bao/tls/tls.key"

	openBaoLabelCluster  = "openbao.org/cluster"
	openBaoLabelRevision = "openbao.org/revision"
)
