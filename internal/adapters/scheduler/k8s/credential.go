package k8s

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/heridotlife/honryu/internal/ports"
)

// credentialSecretPrefix namespaces the home-cluster Secrets Honryu writes to
// hold each registered cluster's credential, keeping them distinct from a
// user's own Secrets.
//
// #nosec G101 -- this is a Secret *name* prefix, not a credential. gosec
// matches the identifier containing "credential" against a string literal;
// the value is the constant "honryu-cluster-" that gets concatenated with a
// cluster name to form a Kubernetes object name. No secret material appears
// in this file -- it is read from and written to the Secret's data keys below.
const credentialSecretPrefix = "honryu-cluster-"

// Secret data keys. The credential is stored field-per-key so a human reading
// the Secret (kubectl) sees labelled parts, not one opaque blob.
const (
	credAPIURLKey     = "api-url"
	credCAKey         = "ca.crt"
	credTokenKey      = "token"
	credClientCertKey = "client.crt"
	credClientKeyKey  = "client.key"
)

// Credential materialization errors. Callers compare with errors.Is.
var (
	ErrCredentialSecretNotFound = errors.New("k8s: cluster credential secret not found")
	ErrCredentialIncomplete     = errors.New("k8s: cluster credential has no api url")
)

// CredentialSecretName is the deterministic home-cluster Secret name holding
// clusterName's credential -- deterministic so materialize (create-or-update)
// and read agree without storing the mapping anywhere. It is what a registry
// entry's SecretRef points at.
func CredentialSecretName(clusterName string) string {
	return credentialSecretPrefix + clusterName
}

// MaterializeCredential writes cred into a home-cluster Secret named secretName,
// creating it or, if it already exists, reconciling its contents in place
// (covers credential rotation). Secret values are never logged.
func MaterializeCredential(ctx context.Context, client kubernetes.Interface, namespace, secretName string, cred Credential) error {
	data := credentialToData(cred)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    map[string]string{managedByLabel: managedByValue},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	_, err := client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("k8s: get cluster credential secret for reconcile: %w", getErr)
		}
		existing.Data = data
		if existing.Labels == nil {
			existing.Labels = map[string]string{}
		}
		existing.Labels[managedByLabel] = managedByValue
		if _, err = client.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("k8s: reconcile cluster credential secret: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: create cluster credential secret: %w", err)
	}
	return nil
}

// ReadCredential reads a materialized Secret back into a Credential, or
// ErrCredentialSecretNotFound if it is absent.
func ReadCredential(ctx context.Context, client kubernetes.Interface, namespace, secretName string) (Credential, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return Credential{}, ErrCredentialSecretNotFound
	}
	if err != nil {
		return Credential{}, fmt.Errorf("k8s: read cluster credential secret: %w", err)
	}
	return Credential{
		APIURL:     string(secret.Data[credAPIURLKey]),
		CACert:     secret.Data[credCAKey],
		Token:      string(secret.Data[credTokenKey]),
		ClientCert: secret.Data[credClientCertKey],
		ClientKey:  secret.Data[credClientKeyKey],
	}, nil
}

// RestConfigFromCredential builds a rest.Config from cred, authenticating with
// its bearer token or client cert/key. It does not reach the network.
func RestConfigFromCredential(cred Credential) (*rest.Config, error) {
	if cred.APIURL == "" {
		return nil, ErrCredentialIncomplete
	}
	cfg := &rest.Config{
		Host: cred.APIURL,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: cred.CACert,
		},
	}
	switch {
	case cred.Token != "":
		cfg.BearerToken = cred.Token
	case len(cred.ClientCert) > 0 && len(cred.ClientKey) > 0:
		cfg.CertData = cred.ClientCert
		cfg.KeyData = cred.ClientKey
	default:
		return nil, ErrNoCredential
	}
	return cfg, nil
}

// RestConfigFromSecret reads a materialized Secret and builds a rest.Config
// from it -- the client factory's single entry point for a non-default cluster.
func RestConfigFromSecret(ctx context.Context, client kubernetes.Interface, namespace, secretName string) (*rest.Config, error) {
	cred, err := ReadCredential(ctx, client, namespace, secretName)
	if err != nil {
		return nil, err
	}
	return RestConfigFromCredential(cred)
}

// HomeCredentialStore reads and writes cluster credentials as Secrets in the
// home cluster, bound to a home client and namespace. It adapts this package's
// Materialize/Read functions to the neutral ports.ClusterCredential the app
// layer (clusterapp) consumes -- satisfying clusterapp.CredentialStore.
type HomeCredentialStore struct {
	client    kubernetes.Interface
	namespace string
}

// NewHomeCredentialStore binds a credential store to the home client + namespace.
func NewHomeCredentialStore(client kubernetes.Interface, namespace string) *HomeCredentialStore {
	return &HomeCredentialStore{client: client, namespace: namespace}
}

// Materialize writes cred into the home-cluster Secret named secretName.
func (s *HomeCredentialStore) Materialize(ctx context.Context, secretName string, cred ports.ClusterCredential) error {
	return MaterializeCredential(ctx, s.client, s.namespace, secretName, credFromPorts(cred))
}

// Delete removes the home-cluster Secret named secretName. A Secret that is
// already gone is not an error -- Delete is a best-effort cleanup.
func (s *HomeCredentialStore) Delete(ctx context.Context, secretName string) error {
	err := s.client.CoreV1().Secrets(s.namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: delete cluster credential secret: %w", err)
	}
	return nil
}

// Read reads the home-cluster Secret named secretName into a neutral credential.
func (s *HomeCredentialStore) Read(ctx context.Context, secretName string) (ports.ClusterCredential, error) {
	c, err := ReadCredential(ctx, s.client, s.namespace, secretName)
	if err != nil {
		return ports.ClusterCredential{}, err
	}
	return ports.ClusterCredential{
		APIURL: c.APIURL, CACert: c.CACert, Token: c.Token, ClientCert: c.ClientCert, ClientKey: c.ClientKey,
	}, nil
}

// ParsePortsKubeconfig validates a self-contained kubeconfig and returns the
// neutral credential -- the KubeconfigParser clusterapp injects.
func ParsePortsKubeconfig(raw []byte) (ports.ClusterCredential, error) {
	c, err := ParseSelfContainedKubeconfig(raw)
	if err != nil {
		return ports.ClusterCredential{}, err
	}
	return ports.ClusterCredential{
		APIURL: c.APIURL, CACert: c.CACert, Token: c.Token, ClientCert: c.ClientCert, ClientKey: c.ClientKey,
	}, nil
}

// credFromPorts converts a neutral credential to this package's Credential.
func credFromPorts(c ports.ClusterCredential) Credential {
	return Credential{APIURL: c.APIURL, CACert: c.CACert, Token: c.Token, ClientCert: c.ClientCert, ClientKey: c.ClientKey}
}

func credentialToData(cred Credential) map[string][]byte {
	data := map[string][]byte{
		credAPIURLKey: []byte(cred.APIURL),
		credCAKey:     cred.CACert,
	}
	if cred.Token != "" {
		data[credTokenKey] = []byte(cred.Token)
	}
	if len(cred.ClientCert) > 0 {
		data[credClientCertKey] = cred.ClientCert
	}
	if len(cred.ClientKey) > 0 {
		data[credClientKeyKey] = cred.ClientKey
	}
	return data
}
