package init

import (
	"testing"

	initmanagerport "github.com/kubebao/openbao-operator/internal/port/initmanager"
)

func TestManagerSatisfiesInitManagerPort(t *testing.T) {
	var _ initmanagerport.Manager = (*Manager)(nil)
}
