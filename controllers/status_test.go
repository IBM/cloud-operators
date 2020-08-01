package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func getObject(ctx context.Context, meta metav1.ObjectMeta, v runtime.Object) error {
	return k8sClient.Get(ctx, types.NamespacedName{
		Name:      meta.Name,
		Namespace: meta.Namespace,
	}, v)
}

type statuser interface {
	runtime.Object
	GetState() string
}

func getStatus(ctx context.Context, meta metav1.ObjectMeta, v statuser) (string, error) {
	err := getObject(ctx, meta, v)
	return v.GetState(), err
}

func verifyStatus(ctx context.Context, meta metav1.ObjectMeta, v statuser, expectedStatus string) func() bool {
	return func() bool {
		status, err := getStatus(ctx, meta, v)
		return err == nil && status == expectedStatus
	}
}
