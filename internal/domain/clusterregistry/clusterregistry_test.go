package clusterregistry_test

import (
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
)

// valid returns a registered operator cluster with every invariant satisfied,
// so each error case below can flip exactly one field.
func valid() clusterregistry.Cluster {
	return clusterregistry.Cluster{
		Name:         "prod-eu",
		APIURL:       "https://prod-eu.example:6443",
		CACert:       "-----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----",
		IngestURL:    "http://honryu-ingest.honryu.svc:8080",
		SidecarImage: "registry.example/honryu-sidecar:1",
		Namespace:    "honryu",
		SecretRef:    "cluster-prod-eu-creds",
		Origin:       clusterregistry.OriginOperator,
	}
}

func TestValidate_Valid(t *testing.T) {
	t.Parallel()

	operator := valid()
	if err := operator.Validate(); err != nil {
		t.Fatalf("Validate(operator) = %v, want nil", err)
	}

	byoc := valid()
	byoc.Origin = clusterregistry.OriginBYOC
	if err := byoc.Validate(); err != nil {
		t.Fatalf("Validate(byoc) = %v, want nil", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*clusterregistry.Cluster)
		wantErr error
	}{
		{"empty name", func(c *clusterregistry.Cluster) { c.Name = "" }, clusterregistry.ErrNameRequired},
		{"blank name", func(c *clusterregistry.Cluster) { c.Name = "   " }, clusterregistry.ErrNameRequired},
		{"empty origin", func(c *clusterregistry.Cluster) { c.Origin = "" }, clusterregistry.ErrOriginUnknown},
		{"unknown origin", func(c *clusterregistry.Cluster) { c.Origin = "vault" }, clusterregistry.ErrOriginUnknown},
		{
			"byoc missing credential reference",
			func(c *clusterregistry.Cluster) {
				c.Origin = clusterregistry.OriginBYOC
				c.SecretRef = ""
			},
			clusterregistry.ErrSecretRefRequired,
		},
		{
			"operator missing credential reference",
			func(c *clusterregistry.Cluster) { c.SecretRef = "  " },
			clusterregistry.ErrSecretRefRequired,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := valid()
			tc.mutate(&c)
			if err := c.Validate(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestIsDefaultName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ref  string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"prod-eu", false},
		{clusterregistry.DefaultName, true},
	}
	for _, tc := range cases {
		if got := clusterregistry.IsDefaultName(tc.ref); got != tc.want {
			t.Errorf("IsDefaultName(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

func TestOriginValues(t *testing.T) {
	t.Parallel()

	if clusterregistry.OriginOperator != "operator" {
		t.Errorf("OriginOperator = %q, want operator", clusterregistry.OriginOperator)
	}
	if clusterregistry.OriginBYOC != "byoc" {
		t.Errorf("OriginBYOC = %q, want byoc", clusterregistry.OriginBYOC)
	}
}
