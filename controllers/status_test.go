package controllers

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func getObject(ctx context.Context, meta metav1.ObjectMeta, v client.Object) error {
	return k8sClient.Get(ctx, types.NamespacedName{
		Name:      meta.Name,
		Namespace: meta.Namespace,
	}, v)
}

type statuser interface {
	client.Object
	GetState() string
}

type messager interface {
	statuser
	GetMessage() string
}

func getStatus(ctx context.Context, meta metav1.ObjectMeta, v statuser) (string, error) {
	err := getObject(ctx, meta, v)
	return v.GetState(), err
}

func verifyStatus(ctx context.Context, t *testing.T, meta metav1.ObjectMeta, v statuser, expectedStatus string) func() bool {
	return func() bool {
		t.Helper()
		status, err := getStatus(ctx, meta, v)
		if err != nil {
			t.Logf("%s: Failed to fetch status: %v", meta.GetName(), err)
			return false
		}
		message := ""
		if msgr, ok := v.(messager); ok {
			message = msgr.GetMessage()
		}
		if status != expectedStatus {
			t.Logf("%s: Expected status %q but got: %q %s", meta.GetName(), expectedStatus, status, message)
		}
		return status == expectedStatus
	}
}
