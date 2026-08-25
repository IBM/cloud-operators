package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ibm/cloud-operators/internal/config"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/clientcmd"
)

const namespace = "default"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	k8sClient, err := getKubeClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, secretErr := k8sClient.CoreV1().Secrets(namespace).Get(ctx, "ibmcloud-operator-secret", metav1.GetOptions{})
	_, configMapErr := k8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, "ibmcloud-operator-defaults", metav1.GetOptions{})
	if secretErr == nil && configMapErr == nil {
		fmt.Println("IBM Cloud Operators configmap and secret already set up. Skipping...")
		return nil
	}

	config := config.MustGetIBMCloud()

	_, err = k8sClient.CoreV1().Secrets(namespace).Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-operator-secret",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "ibmcloud-operator",
			},
		},
		Data: map[string][]byte{
			"api-key": []byte(config.APIKey),
			"region":  []byte(config.Region),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = k8sClient.CoreV1().ConfigMaps(namespace).Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-operator-defaults",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "ibmcloud-operator",
			},
		},
		Data: map[string]string{
			"org":             config.Org,
			"region":          config.Region,
			"resourcegroup":   config.ResourceGroupName,
			"resourcegroupid": config.ResourceGroupID,
			"space":           config.Space,
			"user":            config.UserDisplayName,
		},
	}, metav1.CreateOptions{})
	return err
}

func getKubeClient() (kubernetes.Interface, error) {
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(clientConfig)
}
