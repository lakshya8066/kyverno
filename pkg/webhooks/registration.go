package webhooks

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/golang/glog"
	"github.com/nirmata/kyverno/pkg/config"
	client "github.com/nirmata/kyverno/pkg/dclient"

	admregapi "k8s.io/api/admissionregistration/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	admregclient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	rest "k8s.io/client-go/rest"
)

// WebhookRegistrationClient is client for registration webhooks on cluster
type WebhookRegistrationClient struct {
	registrationClient *admregclient.AdmissionregistrationV1beta1Client
	client             *client.Client
	clientConfig       *rest.Config
	// serverIP should be used if running Kyverno out of clutser
	serverIP string
}

// NewWebhookRegistrationClient creates new WebhookRegistrationClient instance
func NewWebhookRegistrationClient(clientConfig *rest.Config, client *client.Client, serverIP string) (*WebhookRegistrationClient, error) {
	registrationClient, err := admregclient.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	return &WebhookRegistrationClient{
		registrationClient: registrationClient,
		client:             client,
		clientConfig:       clientConfig,
		serverIP:           serverIP,
	}, nil
}

// Register creates admission webhooks configs on cluster
func (wrc *WebhookRegistrationClient) Register() error {
	if wrc.serverIP != "" {
		glog.Infof("Registering webhook with url https://%s\n", wrc.serverIP)
	}
	// For the case if cluster already has this configs
	wrc.Deregister()

	mutatingWebhookConfig, err := wrc.constructMutatingWebhookConfig(wrc.clientConfig)
	if err != nil {
		return err
	}

	_, err = wrc.registrationClient.MutatingWebhookConfigurations().Create(mutatingWebhookConfig)
	if err != nil {
		return err
	}

	validationWebhookConfig, err := wrc.constructValidatingWebhookConfig(wrc.clientConfig)
	if err != nil {
		return err
	}

	_, err = wrc.registrationClient.ValidatingWebhookConfigurations().Create(validationWebhookConfig)
	if err != nil {
		return err
	}

	return nil
}

// Deregister deletes webhook configs from cluster
// This function does not fail on error:
// Register will fail if the config exists, so there is no need to fail on error
func (wrc *WebhookRegistrationClient) Deregister() {
	if wrc.serverIP != "" {
		wrc.registrationClient.MutatingWebhookConfigurations().Delete(config.MutatingWebhookConfigurationDebug, &meta.DeleteOptions{})
		wrc.registrationClient.ValidatingWebhookConfigurations().Delete(config.ValidatingWebhookConfigurationDebug, &meta.DeleteOptions{})
		return
	}

	wrc.registrationClient.MutatingWebhookConfigurations().Delete(config.MutatingWebhookConfigurationName, &meta.DeleteOptions{})
	wrc.registrationClient.ValidatingWebhookConfigurations().Delete(config.ValidatingWebhookConfigurationName, &meta.DeleteOptions{})
}

func (wrc *WebhookRegistrationClient) constructMutatingWebhookConfig(configuration *rest.Config) (*admregapi.MutatingWebhookConfiguration, error) {
	var caData []byte
	// Check if ca is defined in the secret tls-ca
	// assume the key and signed cert have been defined in secret tls.kyverno
	caData = wrc.client.ReadRootCASecret()
	if len(caData) == 0 {
		// load the CA from kubeconfig
		caData = extractCA(configuration)
	}
	if len(caData) == 0 {
		return nil, errors.New("Unable to extract CA data from configuration")
	}

	if wrc.serverIP != "" {
		return wrc.contructDebugMutatingWebhookConfig(caData), nil
	}

	return &admregapi.MutatingWebhookConfiguration{
		ObjectMeta: meta.ObjectMeta{
			Name:   config.MutatingWebhookConfigurationName,
			Labels: config.KubePolicyAppLabels,
			OwnerReferences: []meta.OwnerReference{
				wrc.constructOwner(),
			},
		},
		Webhooks: []admregapi.Webhook{
			constructWebhook(
				config.MutatingWebhookName,
				config.MutatingWebhookServicePath,
				caData),
		},
	}, nil
}

func (wrc *WebhookRegistrationClient) contructDebugMutatingWebhookConfig(caData []byte) *admregapi.MutatingWebhookConfiguration {
	url := fmt.Sprintf("https://%s%s", wrc.serverIP, config.MutatingWebhookServicePath)
	glog.V(3).Infof("Debug MutatingWebhookConfig is registered with url %s\n", url)

	return &admregapi.MutatingWebhookConfiguration{
		ObjectMeta: meta.ObjectMeta{
			Name:   config.MutatingWebhookConfigurationDebug,
			Labels: config.KubePolicyAppLabels,
		},
		Webhooks: []admregapi.Webhook{
			constructDebugWebhook(
				config.MutatingWebhookName,
				url,
				caData),
		},
	}
}

func (wrc *WebhookRegistrationClient) constructValidatingWebhookConfig(configuration *rest.Config) (*admregapi.ValidatingWebhookConfiguration, error) {
	// Check if ca is defined in the secret tls-ca
	// assume the key and signed cert have been defined in secret tls.kyverno
	caData := wrc.client.ReadRootCASecret()
	if len(caData) == 0 {
		// load the CA from kubeconfig
		caData = extractCA(configuration)
	}
	if len(caData) == 0 {
		return nil, errors.New("Unable to extract CA data from configuration")
	}

	if wrc.serverIP != "" {
		return wrc.contructDebugValidatingWebhookConfig(caData), nil
	}

	return &admregapi.ValidatingWebhookConfiguration{
		ObjectMeta: meta.ObjectMeta{
			Name:   config.ValidatingWebhookConfigurationName,
			Labels: config.KubePolicyAppLabels,
			OwnerReferences: []meta.OwnerReference{
				wrc.constructOwner(),
			},
		},
		Webhooks: []admregapi.Webhook{
			constructWebhook(
				config.ValidatingWebhookName,
				config.ValidatingWebhookServicePath,
				caData),
		},
	}, nil
}

func (wrc *WebhookRegistrationClient) contructDebugValidatingWebhookConfig(caData []byte) *admregapi.ValidatingWebhookConfiguration {
	url := fmt.Sprintf("https://%s%s", wrc.serverIP, config.ValidatingWebhookServicePath)
	glog.V(3).Infof("Debug ValidatingWebhookConfig is registered with url %s\n", url)

	return &admregapi.ValidatingWebhookConfiguration{
		ObjectMeta: meta.ObjectMeta{
			Name:   config.ValidatingWebhookConfigurationName,
			Labels: config.KubePolicyAppLabels,
		},
		Webhooks: []admregapi.Webhook{
			constructDebugWebhook(
				config.ValidatingWebhookName,
				url,
				caData),
		},
	}
}

func constructWebhook(name, servicePath string, caData []byte) admregapi.Webhook {
	return admregapi.Webhook{
		Name: name,
		ClientConfig: admregapi.WebhookClientConfig{
			Service: &admregapi.ServiceReference{
				Namespace: config.KubePolicyNamespace,
				Name:      config.WebhookServiceName,
				Path:      &servicePath,
			},
			CABundle: caData,
		},
		Rules: []admregapi.RuleWithOperations{
			admregapi.RuleWithOperations{
				Operations: []admregapi.OperationType{
					admregapi.Create,
				},
				Rule: admregapi.Rule{
					APIGroups: []string{
						"*",
					},
					APIVersions: []string{
						"*",
					},
					Resources: []string{
						"*/*",
					},
				},
			},
		},
	}
}

func constructDebugWebhook(name, url string, caData []byte) admregapi.Webhook {
	return admregapi.Webhook{
		Name: name,
		ClientConfig: admregapi.WebhookClientConfig{
			URL:      &url,
			CABundle: caData,
		},
		Rules: []admregapi.RuleWithOperations{
			admregapi.RuleWithOperations{
				Operations: []admregapi.OperationType{
					admregapi.Create,
				},
				Rule: admregapi.Rule{
					APIGroups: []string{
						"*",
					},
					APIVersions: []string{
						"*",
					},
					Resources: []string{
						"*/*",
					},
				},
			},
		},
	}
}

func (wrc *WebhookRegistrationClient) constructOwner() meta.OwnerReference {
	kubePolicyDeployment, err := wrc.client.GetKubePolicyDeployment()

	if err != nil {
		return meta.OwnerReference{}
	}

	return meta.OwnerReference{
		APIVersion: config.DeploymentAPIVersion,
		Kind:       config.DeploymentKind,
		Name:       kubePolicyDeployment.ObjectMeta.Name,
		UID:        kubePolicyDeployment.ObjectMeta.UID,
	}
}

// ExtractCA used for extraction CA from config
func extractCA(config *rest.Config) (result []byte) {
	fileName := config.TLSClientConfig.CAFile

	if fileName != "" {
		result, err := ioutil.ReadFile(fileName)

		if err != nil {
			return nil
		}

		return result
	}

	return config.TLSClientConfig.CAData
}