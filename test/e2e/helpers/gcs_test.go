package helpers

import (
	"testing"

	"k8s.io/client-go/rest"
)

func TestFakeGCSBucketProxyURL(t *testing.T) {
	t.Parallel()

	got, err := fakeGCSBucketProxyURL(
		&rest.Config{Host: "https://127.0.0.1:6443/"},
		"gcs",
		"http://fake-gcs-server.gcs.svc.cluster.local:4443",
		"test-project",
	)
	if err != nil {
		t.Fatalf("fakeGCSBucketProxyURL() error = %v", err)
	}

	want := "https://127.0.0.1:6443/api/v1/namespaces/gcs/services/http:fake-gcs-server:4443/proxy/storage/v1/b?project=test-project"
	if got != want {
		t.Fatalf("fakeGCSBucketProxyURL() = %q, want %q", got, want)
	}
}
