package config

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRenderSelfInitStanzas_StructuredPrefixRequiresStructuredFields_EvenWithRawData(t *testing.T) {
	file := hclwrite.NewEmptyFile()

	err := renderSelfInitStanzas(file.Body(), []openbaov1alpha1.SelfInitRequest{
		{
			Name:      "enable-audit",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/audit/stdout",
			Data:      &apiextensionsv1.JSON{Raw: []byte(`{"type":"file"}`)},
		},
	})

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	want := `audit device request "enable-audit" at path "sys/audit/stdout" must use structured auditDevice field, not raw data`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestRenderSelfInitStanzas_UnknownPrefix_UsesRawData(t *testing.T) {
	file := hclwrite.NewEmptyFile()

	err := renderSelfInitStanzas(file.Body(), []openbaov1alpha1.SelfInitRequest{
		{
			Name:      "configure-jwt",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/config",
			Data:      &apiextensionsv1.JSON{Raw: []byte(`{"foo":"bar"}`)},
		},
	})
	if err != nil {
		t.Fatalf("renderSelfInitStanzas() error = %v", err)
	}

	got := string(file.Bytes())
	if !strings.Contains(got, `path      = "auth/jwt/config"`) {
		t.Fatalf("expected output to include request path, got:\n%s", got)
	}
	if !strings.Contains(got, "data {") || !strings.Contains(got, `foo = "bar"`) {
		t.Fatalf("expected output to include data block, got:\n%s", got)
	}
}

func TestRenderSelfInitStanzas_KnownPrefix_IgnoresRawDataAndUsesStructured(t *testing.T) {
	file := hclwrite.NewEmptyFile()

	err := renderSelfInitStanzas(file.Body(), []openbaov1alpha1.SelfInitRequest{
		{
			Name:      "enable-jwt",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/auth/jwt",
			AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
				Type: "jwt",
			},
			Data: &apiextensionsv1.JSON{Raw: []byte(`{"type":"kubernetes"}`)},
		},
	})
	if err != nil {
		t.Fatalf("renderSelfInitStanzas() error = %v", err)
	}

	got := string(file.Bytes())
	if !strings.Contains(got, `type = "jwt"`) {
		t.Fatalf("expected output to use structured auth method type, got:\n%s", got)
	}
	if strings.Contains(got, "kubernetes") {
		t.Fatalf("expected raw data to be ignored for structured prefixes, got:\n%s", got)
	}
}

func TestResolveSelfInitRequestStructuredData_PicksMostSpecificPrefix(t *testing.T) {
	original := selfInitRequestDataHandlers
	selfInitRequestDataHandlers = []selfInitRequestDataHandlerRegistration{
		{
			Prefix: "sys/",
			Handler: func(_ openbaov1alpha1.SelfInitRequest) (cty.Value, error) {
				return cty.StringVal("broad"), nil
			},
		},
		{
			Prefix: "sys/auth/",
			Handler: func(_ openbaov1alpha1.SelfInitRequest) (cty.Value, error) {
				return cty.StringVal("specific"), nil
			},
		},
	}
	t.Cleanup(func() { selfInitRequestDataHandlers = original })

	val, handled, err := resolveSelfInitRequestStructuredData(openbaov1alpha1.SelfInitRequest{
		Name: "test",
		Path: "sys/auth/jwt",
	})
	if err != nil {
		t.Fatalf("resolveSelfInitRequestStructuredData() error = %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true, got false")
	}
	if val.Type() != cty.String || val.AsString() != "specific" {
		t.Fatalf("expected most specific handler, got %s", val.GoString())
	}
}
