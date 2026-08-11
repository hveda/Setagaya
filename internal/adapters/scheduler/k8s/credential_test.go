package k8s

import (
	"bytes"
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCredentialSecretName_Deterministic(t *testing.T) {
	t.Parallel()
	if got := CredentialSecretName("prod-eu"); got != "honryu-cluster-prod-eu" {
		t.Fatalf("CredentialSecretName = %q", got)
	}
}

func TestMaterializeAndReadCredential_RoundTrips(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	name := CredentialSecretName("prod-eu")

	cred := Credential{
		APIURL: "https://api.example:6443",
		CACert: []byte("ca-pem"),
		Token:  "sa-token-abc",
	}
	if err := MaterializeCredential(ctx, client, "honryu", name, cred); err != nil {
		t.Fatalf("MaterializeCredential: %v", err)
	}
	got, err := ReadCredential(ctx, client, "honryu", name)
	if err != nil {
		t.Fatalf("ReadCredential: %v", err)
	}
	if got.APIURL != cred.APIURL || !bytes.Equal(got.CACert, cred.CACert) || got.Token != cred.Token {
		t.Fatalf("ReadCredential = %+v, want %+v", got, cred)
	}

	// The materialized Secret is Opaque and labelled managed-by=honryu.
	secret, err := client.CoreV1().Secrets("honryu").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get secret: %v", err)
	}
	if secret.Labels[managedByLabel] != managedByValue {
		t.Errorf("Secret missing managed-by label: %v", secret.Labels)
	}
}

// A second materialize with rotated values must update the existing Secret in
// place, not fail on AlreadyExists.
func TestMaterializeCredential_ReconcilesInPlace(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	name := CredentialSecretName("prod-eu")

	if err := MaterializeCredential(ctx, client, "honryu", name, Credential{APIURL: "https://a", CACert: []byte("ca"), Token: "old"}); err != nil {
		t.Fatalf("MaterializeCredential (first): %v", err)
	}
	if err := MaterializeCredential(ctx, client, "honryu", name, Credential{APIURL: "https://a", CACert: []byte("ca"), Token: "rotated"}); err != nil {
		t.Fatalf("MaterializeCredential (rotate): %v", err)
	}

	got, err := ReadCredential(ctx, client, "honryu", name)
	if err != nil {
		t.Fatalf("ReadCredential: %v", err)
	}
	if got.Token != "rotated" {
		t.Fatalf("Token = %q after reconcile, want rotated", got.Token)
	}

	// Exactly one Secret exists -- reconcile updated, did not duplicate.
	list, err := client.CoreV1().Secrets("honryu").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("secret count = %d, want 1", len(list.Items))
	}
}

func TestReadCredential_NotFound(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset()
	if _, err := ReadCredential(context.Background(), client, "honryu", "nope"); !errors.Is(err, ErrCredentialSecretNotFound) {
		t.Fatalf("ReadCredential(missing) = %v, want ErrCredentialSecretNotFound", err)
	}
}

func TestRestConfigFromCredential(t *testing.T) {
	t.Parallel()

	t.Run("token", func(t *testing.T) {
		cfg, err := RestConfigFromCredential(Credential{APIURL: "https://a", CACert: []byte("ca"), Token: "t"})
		if err != nil {
			t.Fatalf("RestConfigFromCredential: %v", err)
		}
		if cfg.Host != "https://a" || cfg.BearerToken != "t" || !bytes.Equal(cfg.CAData, []byte("ca")) {
			t.Fatalf("rest.Config = %+v", cfg)
		}
	})

	t.Run("client cert", func(t *testing.T) {
		cfg, err := RestConfigFromCredential(Credential{APIURL: "https://a", CACert: []byte("ca"), ClientCert: []byte("crt"), ClientKey: []byte("key")})
		if err != nil {
			t.Fatalf("RestConfigFromCredential: %v", err)
		}
		if !bytes.Equal(cfg.CertData, []byte("crt")) || !bytes.Equal(cfg.KeyData, []byte("key")) {
			t.Fatalf("rest.Config TLS = %+v", cfg.TLSClientConfig)
		}
	})

	t.Run("no api url", func(t *testing.T) {
		if _, err := RestConfigFromCredential(Credential{Token: "t"}); !errors.Is(err, ErrCredentialIncomplete) {
			t.Fatalf("err = %v, want ErrCredentialIncomplete", err)
		}
	})

	t.Run("no credential", func(t *testing.T) {
		if _, err := RestConfigFromCredential(Credential{APIURL: "https://a", CACert: []byte("ca")}); !errors.Is(err, ErrNoCredential) {
			t.Fatalf("err = %v, want ErrNoCredential", err)
		}
	})
}

func TestRestConfigFromSecret_RoundTrips(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	name := CredentialSecretName("prod-eu")
	if err := MaterializeCredential(ctx, client, "honryu", name, Credential{APIURL: "https://a", CACert: []byte("ca"), Token: "t"}); err != nil {
		t.Fatalf("MaterializeCredential: %v", err)
	}
	cfg, err := RestConfigFromSecret(ctx, client, "honryu", name)
	if err != nil {
		t.Fatalf("RestConfigFromSecret: %v", err)
	}
	if cfg.Host != "https://a" || cfg.BearerToken != "t" {
		t.Fatalf("rest.Config = %+v", cfg)
	}
}
