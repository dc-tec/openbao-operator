package certs

import (
	"testing"
	"time"
)

func TestTLSMetrics_NoPanic(t *testing.T) {
	m := newTLSMetrics("ns", "name")

	now := time.Now()
	m.setServerCertExpiry(now, "OperatorManaged")
	m.setServerCertExpiry(now.Add(24*time.Hour), "External")
	m.incrementRotation()
}
