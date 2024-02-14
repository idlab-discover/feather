package root

import (
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/authenticatorfactory"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/kubernetes"
	"time"
)

type authWrapper struct {
	authenticator.Request
	authorizer.RequestAttributesGetter
	authorizer.Authorizer
}

// WebhookAuth creates an Auth suitable to use with kubelet webhook auth.
// You must provide a CA provider to the authentication config, otherwise mTLS is disabled.
func WebhookAuth(client kubernetes.Interface, nodeName string, opts ...nodeutil.WebhookAuthOption) (nodeutil.Auth, error) {
	cfg := nodeutil.WebhookAuthConfig{
		AuthnConfig: authenticatorfactory.DelegatingAuthenticatorConfig{
			CacheTTL:            2 * time.Minute, // default taken from k8s.io/kubernetes/pkg/kubelet/apis/config/v1beta1
			WebhookRetryBackoff: options.DefaultAuthWebhookRetryBackoff(),
		},
		AuthzConfig: authorizerfactory.DelegatingAuthorizerConfig{
			AllowCacheTTL:       5 * time.Minute,  // default taken from k8s.io/kubernetes/pkg/kubelet/apis/config/v1beta1
			DenyCacheTTL:        30 * time.Second, // default taken from k8s.io/kubernetes/pkg/kubelet/apis/config/v1beta1
			WebhookRetryBackoff: options.DefaultAuthWebhookRetryBackoff(),
		},
	}

	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}

	cfg.AuthnConfig.TokenAccessReviewClient = client.AuthenticationV1()
	cfg.AuthzConfig.SubjectAccessReviewClient = client.AuthorizationV1()

	authn, _, err := cfg.AuthnConfig.New()
	if err != nil {
		return nil, err
	}

	authz, err := cfg.AuthzConfig.New()
	if err != nil {
		return nil, err
	}
	return &authWrapper{
		Request:                 authn,
		RequestAttributesGetter: nodeutil.NodeRequestAttr{NodeName: nodeName},
		Authorizer:              authz,
	}, nil
}
