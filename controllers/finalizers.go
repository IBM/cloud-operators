package controllers

import (
	"strings"

	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
)

// containsFinalizer checks if the instance contains service finalizer
func containsFinalizer(instance *ibmcloudv1beta1.Binding) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, bindingFinalizer) {
			return true
		}
	}
	return false
}

// deleteFinalizer delete service finalizer
func deleteFinalizer(instance *ibmcloudv1beta1.Binding) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == bindingFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}
