package kommons

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	certs "github.com/flanksource/commons/certs"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func CreateKubeConfig(clusterName string, ca certs.CertificateAuthority, endpoint string, group string, user string, expiry time.Duration) ([]byte, error) {
	contextName := fmt.Sprintf("%s@%s", user, clusterName)
	cert := certs.NewCertificateBuilder(user).Organization(group).Client().Certificate
	if cert.X509.PublicKey == nil && cert.PrivateKey != nil {
		cert.X509.PublicKey = cert.PrivateKey.Public()
	}
	signed, err := ca.Sign(cert.X509, expiry)
	if err != nil {
		return nil, fmt.Errorf("createKubeConfig: failed to sign certificate: %v", err)
	}
	cert = &certs.Certificate{
		X509:       signed,
		PrivateKey: cert.PrivateKey,
	}

	if !strings.Contains(endpoint, ":") {
		endpoint = endpoint + ":6443"
	}
	cfg := api.Config{
		Clusters: map[string]*api.Cluster{
			clusterName: {
				Server:                "https://" + endpoint,
				InsecureSkipTLSVerify: true,
				// The CA used for signing the client certificate is not the same as the
				// as the CA (kubernetes-ca) that signed the api-server cert. The kubernetes-ca
				// is ephemeral.
				// TODO dynamically download CA from master server
				// CertificateAuthorityData: []byte(platform.Certificates.CA.X509),
			},
		},
		Contexts: map[string]*api.Context{
			contextName: {
				Cluster:   clusterName,
				AuthInfo:  contextName,
				Namespace: "kube-system",
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			contextName: {
				ClientKeyData:         cert.EncodedPrivateKey(),
				ClientCertificateData: cert.EncodedCertificate(),
			},
		},
		CurrentContext: contextName,
	}

	return clientcmd.Write(cfg)
}

func CreateOIDCKubeConfig(clusterName string, ca certs.CertificateAuthority, endpoint, idpURL, idToken, accessToken, refreshToken string) ([]byte, error) {
	if !strings.HasPrefix("https://", endpoint) {
		endpoint = "https://" + endpoint
	}

	if !strings.HasPrefix("https://", idpURL) {
		idpURL = "https://" + idpURL
	}
	cfg := api.Config{
		Clusters: map[string]*api.Cluster{
			clusterName: {
				Server:                endpoint + ":6443",
				InsecureSkipTLSVerify: true,
			},
		},
		Contexts: map[string]*api.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: "sso@" + clusterName,
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			"sso@" + clusterName: {
				AuthProvider: &api.AuthProviderConfig{
					Name: "oidc",
					Config: map[string]string{
						"client-id":                      "kubernetes",
						"client-secret":                  "ZXhhbXBsZS1hcHAtc2VjcmV0",
						"extra-scopes":                   "offline_access openid profile email groups",
						"idp-certificate-authority-data": base64.StdEncoding.EncodeToString(ca.GetPublicChain()[0].EncodedCertificate()),
						"idp-issuer-url":                 idpURL,
						"id-token":                       idToken,
						"access-token":                   accessToken,
						"refresh-token":                  refreshToken,
					},
				},
			},
		},
		CurrentContext: clusterName,
	}

	return clientcmd.Write(cfg)
}

// CreateMultiKubeConfig creates a kubeconfig file contents for a map of
// cluster name -> cluster API endpoint hosts, all with a shared
// user name, group and cert expiry.
// NOTE: these clusters all need to share the same plaform CA
func CreateMultiKubeConfig(ca certs.CertificateAuthority, clusters map[string]string, group string, user string, expiry time.Duration) ([]byte, error) {
	if len(clusters) < 1 {
		return []byte{}, fmt.Errorf("CreateMultiKubeConfig failed since it was given an empty cluster map")
	}
	cfg := api.Config{
		Clusters:       map[string]*api.Cluster{},
		Contexts:       map[string]*api.Context{},
		AuthInfos:      map[string]*api.AuthInfo{},
		CurrentContext: "",
	}
	for clusterName, endpoint := range clusters {
		cert := certs.NewCertificateBuilder(user).Organization(group).Client().Certificate
		if cert.X509.PublicKey == nil && cert.PrivateKey != nil {
			cert.X509.PublicKey = cert.PrivateKey.Public()
		}
		signed, err := ca.Sign(cert.X509, expiry)
		if err != nil {
			return nil, fmt.Errorf("createKubeConfig: failed to sign certificate: %v", err)
		}
		cert = &certs.Certificate{
			X509:       signed,
			PrivateKey: cert.PrivateKey,
		}
		cfg.Clusters[clusterName] = &api.Cluster{
			Server:                endpoint,
			InsecureSkipTLSVerify: true,
		}
		context := fmt.Sprintf("%s@%s", user, clusterName)
		cfg.Contexts[clusterName] = &api.Context{
			Cluster:   clusterName,
			AuthInfo:  context,
			Namespace: "kube-system", //TODO: verify
		}
		cfg.AuthInfos[context] = &api.AuthInfo{
			ClientKeyData:         cert.EncodedPrivateKey(),
			ClientCertificateData: cert.EncodedCertificate(),
		}
	}
	return clientcmd.Write(cfg)
}
