package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getConfigMap gets the ubernetes configmap of the given name.
func getConfigMap(ctx context.Context, r client.Client, logt logr.Logger, cmname string, fallback bool, namespace string) (*v1.ConfigMap, error) {
	log := logt.WithName(fmt.Sprintf("%s/%s", namespace, cmname))
	log.V(5).Info("getting configmap")

	var cm v1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cmname}, &cm); err != nil {
		if namespace != "default" && fallback {
			if err := r.Get(ctx, client.ObjectKey{Namespace: "default", Name: cmname}, &cm); err != nil {
				log.V(5).Info("configmap not found")
				return nil, err
			}
		} else {
			log.V(5).Info("configmap not found")
			return nil, err
		}
	}
	log.V(5).Info("configmap found")
	return &cm, nil
}

// getConfigMapValue gets the value of the configmap of the given name in the given namespace. If not found and fallback is true, check default namespace
func getConfigMapValue(ctx context.Context, r client.Client, logt logr.Logger, name string, key string, fallback bool, namespace string) (string, error) {
	cm, err := getConfigMap(ctx, r, logt, name, fallback, namespace)
	if err != nil {
		return "", err
	}

	return cm.Data[key], nil
}
