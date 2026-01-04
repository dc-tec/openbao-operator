package openbaocluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSingleTenantModeField(t *testing.T) {
	t.Run("SingleTenantMode defaults to false", func(t *testing.T) {
		r := &OpenBaoClusterReconciler{}
		assert.False(t, r.SingleTenantMode, "SingleTenantMode should default to false")
	})

	t.Run("SingleTenantMode can be set to true", func(t *testing.T) {
		r := &OpenBaoClusterReconciler{SingleTenantMode: true}
		assert.True(t, r.SingleTenantMode, "SingleTenantMode should be settable to true")
	})
}
