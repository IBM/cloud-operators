package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ibmcloudv1 "github.com/ibm/cloud-operators/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getSecret takes a name and namespace for a Binding and returns the corresponding secret
func getSecret(r client.Client, binding *ibmcloudv1.Binding) (*v1.Secret, error) {
	secretName := binding.Name
	if binding.Spec.SecretName != "" {
		secretName = binding.Spec.SecretName
	}
	secretInstance := &v1.Secret{}
	err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: binding.ObjectMeta.Namespace}, secretInstance)
	if err != nil {
		return &v1.Secret{}, err
	}
	return secretInstance, nil
}

// getKubeSecret gets a kubernetes secret
func getKubeSecret(ctx context.Context, r client.Client, logt logr.Logger, secretname string, fallback bool, namespace string) (*v1.Secret, error) {
	log := logt.WithName(fmt.Sprintf("%s/%s", namespace, secretname))
	log.V(5).Info("getting secret")

	var secret v1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretname}, &secret); err != nil {
		if namespace != "default" && fallback {
			if err := r.Get(ctx, client.ObjectKey{Namespace: "default", Name: secretname}, &secret); err != nil {
				log.V(5).Info("secret not found")
				return nil, err
			}
		} else {
			log.V(5).Info("secret not found")
			return nil, err
		}
	}
	log.V(5).Info("secret found")
	return &secret, nil
}

// getKubeSecretValue gets the value of a secret in the given namespace. If not found and fallback is true, check default namespace
func getKubeSecretValue(ctx context.Context, r client.Client, logt logr.Logger, name string, key string, fallback bool, namespace string) ([]byte, error) {
	secret, err := getKubeSecret(ctx, r, logt, name, fallback, namespace)
	if err != nil {
		return nil, err
	}

	return secret.Data[key], nil
}
