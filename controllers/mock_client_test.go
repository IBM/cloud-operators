package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockClient interface {
	client.Client
	client.WithWatch

	LastCreate() client.Object
	LastDelete() client.Object
	LastDeleteAllOf() client.Object
	LastPatch() client.Object
	LastStatusPatch() client.Object
	LastStatusUpdate() client.Object
	LastUpdate() client.Object
}

type mockClient struct {
	client.Client
	statusWriter *mockStatusWriter
	MockConfig

	lastCreate       client.Object
	lastDelete       client.Object
	lastUpdate       client.Object
	lastPatch        client.Object
	lastDeleteAllOf  client.Object
	lastStatusUpdate client.Object
	lastStatusPatch  client.Object
}

type mockStatusWriter struct {
	*mockClient // pointer to parent mockClient
}

type MockConfig struct {
	CreateErr       error
	DeleteAllOfErr  error
	DeleteErr       error
	PatchErr        error
	StatusPatchErr  error
	StatusUpdateErr error
	UpdateErr       error

	ErrChan chan error
}

func newMockClient(client client.Client, config MockConfig) MockClient {
	m := &mockClient{
		Client:     client,
		MockConfig: config,
	}
	m.statusWriter = &mockStatusWriter{m}
	return m
}

func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	m.lastCreate = obj.DeepCopyObject().(client.Object)
	return m.CreateErr
}

func (m *mockClient) LastCreate() client.Object {
	return m.lastCreate
}

func (m *mockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	m.lastDelete = obj.DeepCopyObject().(client.Object)
	return m.DeleteErr
}

func (m *mockClient) LastDelete() client.Object {
	return m.lastDelete.DeepCopyObject().(client.Object)
}

func (m *mockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	m.lastUpdate = obj.DeepCopyObject().(client.Object)
	return m.UpdateErr
}

func (m *mockClient) LastUpdate() client.Object {
	return m.lastUpdate
}

func (m *mockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	m.lastPatch = obj.DeepCopyObject().(client.Object)
	return m.PatchErr
}

func (m *mockClient) LastPatch() client.Object {
	return m.lastPatch
}

func (m *mockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	m.lastDeleteAllOf = obj.DeepCopyObject().(client.Object)
	return m.DeleteAllOfErr
}

func (m *mockClient) LastDeleteAllOf() client.Object {
	return m.lastDeleteAllOf
}

func (m *mockClient) Status() client.StatusWriter {
	return m.statusWriter
}

func (s *mockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	s.lastStatusUpdate = obj.DeepCopyObject().(client.Object)
	if s.ErrChan != nil {
		err := <-s.ErrChan
		return err
	}
	return s.StatusUpdateErr

}

func (s *mockStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	panic("implement me")
}

func (m *mockClient) LastStatusUpdate() client.Object {
	return m.lastStatusUpdate
}

func (s *mockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	s.lastStatusPatch = obj.DeepCopyObject().(client.Object)
	return s.StatusPatchErr
}

func (m *mockClient) LastStatusPatch() client.Object {
	return m.lastStatusPatch
}

func (m *mockClient) Watch(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	panic("not implemented")
}
