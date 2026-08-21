//go:build e2e
// +build e2e

package framework

import (
	"reflect"
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
)

func TestParseAPIServerEndpointIPs(t *testing.T) {
	t.Parallel()

	got, err := ParseAPIServerEndpointIPs(" 192.168.97.3,192.168.97.2,192.168.97.3 ")
	if err != nil {
		t.Fatalf("ParseAPIServerEndpointIPs() error = %v", err)
	}
	want := []string{"192.168.97.2", "192.168.97.3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseAPIServerEndpointIPs() = %v, want %v", got, want)
	}
}

func TestParseAPIServerEndpointIPsRejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	if _, err := ParseAPIServerEndpointIPs("not-an-ip"); err == nil {
		t.Fatal("ParseAPIServerEndpointIPs() error = nil, want invalid address error")
	}
}

func TestAPIServerEndpointIPsFromEndpointSlices(t *testing.T) {
	t.Parallel()

	endpointSlices := []discoveryv1.EndpointSlice{
		{
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"192.168.97.3", "192.168.97.2"}},
			},
		},
		{
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"192.168.97.2"}},
			},
		},
	}

	got, err := APIServerEndpointIPsFromEndpointSlices(endpointSlices)
	if err != nil {
		t.Fatalf("APIServerEndpointIPsFromEndpointSlices() error = %v", err)
	}
	want := []string{"192.168.97.2", "192.168.97.3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("APIServerEndpointIPsFromEndpointSlices() = %v, want %v", got, want)
	}
}

func TestAPIServerEndpointIPsFromEndpointSlicesRequiresAddress(t *testing.T) {
	t.Parallel()

	if _, err := APIServerEndpointIPsFromEndpointSlices(nil); err == nil {
		t.Fatal("APIServerEndpointIPsFromEndpointSlices() error = nil, want no endpoint IPs error")
	}
}
