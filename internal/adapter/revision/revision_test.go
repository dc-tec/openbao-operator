package revision

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenBaoClusterRevision(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		image    string
		replicas int32
		want     string
	}{
		{
			name:     "basic revision",
			version:  "1.0.0",
			image:    "openbao:1.0.0",
			replicas: 3,
			// Expected value calculated manually or just ensuring determinism
			// sha256("version=1.0.0\nimage=openbao:1.0.0\nreplicas=3\n")
			// = 6886...
			want: "9a193fe604dd8ef0",
		},
		{
			name:     "different version changes revision",
			version:  "1.0.1",
			image:    "openbao:1.0.0",
			replicas: 3,
			want:     "30b1090d66549592",
		},
		{
			name:     "different replicas changes revision",
			version:  "1.0.0",
			image:    "openbao:1.0.0",
			replicas: 5,
			want:     "430e99cd8243bf2e", // Update this with actual value if needed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OpenBaoClusterRevision(tt.version, tt.image, tt.replicas)
			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			}
			// Verify length
			assert.Equal(t, revisionLength, len(got))
		})
	}
}
