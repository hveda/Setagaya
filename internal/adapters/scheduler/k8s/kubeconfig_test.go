package k8s

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// tokenKubeconfig is a self-contained, provider-neutral kubeconfig template.
func tokenKubeconfig() []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    token: sa-token-abc123
`, b64("dummy-ca-pem")))
}

func certKubeconfig() []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    client-certificate-data: %s
    client-key-data: %s
`, b64("dummy-ca-pem"), b64("client-cert-pem"), b64("client-key-pem")))
}

func execKubeconfig(command string) []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: %s
`, b64("dummy-ca-pem"), command))
}

func authProviderKubeconfig() []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    auth-provider:
      name: gcp
`, b64("dummy-ca-pem")))
}

func TestParseSelfContainedKubeconfig_AcceptsToken(t *testing.T) {
	t.Parallel()
	cred, err := ParseSelfContainedKubeconfig(tokenKubeconfig())
	if err != nil {
		t.Fatalf("ParseSelfContainedKubeconfig: %v", err)
	}
	if cred.APIURL != "https://api.example:6443" {
		t.Errorf("APIURL = %q", cred.APIURL)
	}
	if !bytes.Equal(cred.CACert, []byte("dummy-ca-pem")) {
		t.Errorf("CACert = %q, want decoded dummy-ca-pem", cred.CACert)
	}
	if cred.Token != "sa-token-abc123" {
		t.Errorf("Token = %q", cred.Token)
	}
	if len(cred.ClientCert) != 0 || len(cred.ClientKey) != 0 {
		t.Errorf("unexpected client cert/key on a token config")
	}
}

func TestParseSelfContainedKubeconfig_AcceptsClientCert(t *testing.T) {
	t.Parallel()
	cred, err := ParseSelfContainedKubeconfig(certKubeconfig())
	if err != nil {
		t.Fatalf("ParseSelfContainedKubeconfig: %v", err)
	}
	if !bytes.Equal(cred.ClientCert, []byte("client-cert-pem")) || !bytes.Equal(cred.ClientKey, []byte("client-key-pem")) {
		t.Errorf("client cert/key not extracted: %q / %q", cred.ClientCert, cred.ClientKey)
	}
	if cred.Token != "" {
		t.Errorf("Token = %q, want empty on a cert config", cred.Token)
	}
}

func TestParseSelfContainedKubeconfig_Rejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     []byte
		wantErr error
	}{
		{"gke-exec", execKubeconfig("gke-gcloud-auth-plugin"), ErrExecAuthUnsupported},
		{"eks-exec", execKubeconfig("aws"), ErrExecAuthUnsupported},
		{"auth-provider", authProviderKubeconfig(), ErrAuthProviderUnsupported},
		{"unparseable", []byte("::: not yaml :::"), ErrKubeconfigParse},
		{
			"no current context",
			[]byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    token: t
`, b64("ca"))),
			ErrNoCurrentContext,
		},
		{
			"external file reference (token-file)",
			[]byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    tokenFile: /var/run/secrets/token
`, b64("ca"))),
			ErrExternalFileReference,
		},
		{
			"external file reference (ca path)",
			[]byte(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority: /etc/ca.crt
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    token: t
`),
			ErrExternalFileReference,
		},
		{
			"no embedded CA",
			[]byte(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user:
    token: t
`),
			ErrNoCA,
		},
		{
			"no credential",
			[]byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://api.example:6443
    certificate-authority-data: %s
contexts:
- name: ctx
  context:
    cluster: c
    user: u
users:
- name: u
  user: {}
`, b64("ca"))),
			ErrNoCredential,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseSelfContainedKubeconfig(tc.raw); !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseSelfContainedKubeconfig(%s) err = %v, want %v", tc.name, err, tc.wantErr)
			}
		})
	}
}
