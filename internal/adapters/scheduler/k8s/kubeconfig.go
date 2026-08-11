package k8s

import (
	"errors"
	"fmt"

	"k8s.io/client-go/tools/clientcmd"
)

// Credential is the self-contained, provider-neutral credential extracted from
// a kubeconfig: enough to build a rest.Config with no exec plugin, no
// auth-provider, and no cloud SDK. Exactly the shape the Secret materializer
// (task 91) persists and the client factory (task 95) consumes.
type Credential struct {
	APIURL string
	CACert []byte // embedded CA, PEM
	// One of a bearer token or a client cert/key pair authenticates the client.
	Token      string
	ClientCert []byte // PEM
	ClientKey  []byte // PEM
}

// Kubeconfig validation errors. Callers compare with errors.Is.
var (
	ErrKubeconfigParse         = errors.New("kubeconfig: cannot parse")
	ErrNoCurrentContext        = errors.New("kubeconfig: no current-context set")
	ErrContextIncomplete       = errors.New("kubeconfig: current context is missing its cluster or user")
	ErrExecAuthUnsupported     = errors.New("kubeconfig: exec-based auth is not supported; provide a static bearer token or client certificate")
	ErrAuthProviderUnsupported = errors.New("kubeconfig: auth-provider plugins are not supported; provide a static bearer token or client certificate")
	ErrExternalFileReference   = errors.New("kubeconfig: references an external file; the config must be self-contained (embedded CA, token, or client cert)")
	ErrNoServer                = errors.New("kubeconfig: cluster has no server URL")
	ErrNoCA                    = errors.New("kubeconfig: cluster has no embedded CA certificate")
	ErrNoCredential            = errors.New("kubeconfig: no bearer token or client certificate found")
)

// ParseSelfContainedKubeconfig validates that raw is a provider-neutral,
// self-contained kubeconfig and extracts its credential. It rejects any
// current context whose user authenticates via an exec plugin (GKE/EKS/AKS) or
// a legacy auth-provider, and any config that references credentials by
// external file path rather than embedding them -- neither would work from the
// control-plane pod, and both defeat provider-neutrality.
func ParseSelfContainedKubeconfig(raw []byte) (Credential, error) {
	cfg, err := clientcmd.Load(raw)
	if err != nil {
		return Credential{}, fmt.Errorf("%w: %v", ErrKubeconfigParse, err)
	}
	if cfg.CurrentContext == "" {
		return Credential{}, ErrNoCurrentContext
	}
	kctx, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok || kctx.Cluster == "" || kctx.AuthInfo == "" {
		return Credential{}, ErrContextIncomplete
	}
	cluster, clusterOK := cfg.Clusters[kctx.Cluster]
	user, userOK := cfg.AuthInfos[kctx.AuthInfo]
	if !clusterOK || !userOK {
		return Credential{}, ErrContextIncomplete
	}

	// Provider-neutral auth only: reject exec plugins and auth-providers, the
	// two dynamic-credential mechanisms that need a cloud SDK or plugin binary.
	if user.Exec != nil {
		return Credential{}, ErrExecAuthUnsupported
	}
	if user.AuthProvider != nil {
		return Credential{}, ErrAuthProviderUnsupported
	}

	// Self-contained only: reject every path-based reference -- the file would
	// not exist in the control-plane pod.
	if cluster.CertificateAuthority != "" || user.TokenFile != "" ||
		user.ClientCertificate != "" || user.ClientKey != "" {
		return Credential{}, ErrExternalFileReference
	}

	if cluster.Server == "" {
		return Credential{}, ErrNoServer
	}
	if len(cluster.CertificateAuthorityData) == 0 {
		// An embedded CA is required; InsecureSkipTLSVerify is not an accepted
		// substitute for a load-testing platform reaching arbitrary clusters.
		return Credential{}, ErrNoCA
	}

	cred := Credential{
		APIURL:     cluster.Server,
		CACert:     cluster.CertificateAuthorityData,
		Token:      user.Token,
		ClientCert: user.ClientCertificateData,
		ClientKey:  user.ClientKeyData,
	}
	hasToken := cred.Token != ""
	hasCert := len(cred.ClientCert) > 0 && len(cred.ClientKey) > 0
	if !hasToken && !hasCert {
		return Credential{}, ErrNoCredential
	}
	return cred, nil
}
