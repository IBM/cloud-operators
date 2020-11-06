package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockClient interface {
	client.Client

	LastCreate() runtime.Object
	LastDelete() runtime.Object
	LastDeleteAllOf() runtime.Object
	LastPatch() runtime.Object
	LastStatusPatch() runtime.Object
	LastStatusUpdate() runtime.Object
	LastUpdate() runtime.Object
}

type mockClient struct {
	client.Client
	statusWriter *mockStatusWriter
	MockConfig

	lastCreate       runtime.Object
	lastDelete       runtime.Object
	lastUpdate       runtime.Object
	lastPatch        runtime.Object
	lastDeleteAllOf  runtime.Object
	lastStatusUpdate runtime.Object
	lastStatusPatch  runtime.Object
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

func (m *mockClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	m.lastCreate = obj.DeepCopyObject()
	return m.CreateErr
}

func (m *mockClient) LastCreate() runtime.Object {
	return m.lastCreate
}

func (m *mockClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	m.lastDelete = obj.DeepCopyObject()
	return m.DeleteErr
}

func (m *mockClient) LastDelete() runtime.Object {
	return m.lastDelete
}

func (m *mockClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	m.lastUpdate = obj.DeepCopyObject()
	return m.UpdateErr
}

func (m *mockClient) LastUpdate() runtime.Object {
	return m.lastUpdate
}

func (m *mockClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	m.lastPatch = obj.DeepCopyObject()
	return m.PatchErr
}

func (m *mockClient) LastPatch() runtime.Object {
	return m.lastPatch
}

func (m *mockClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	m.lastDeleteAllOf = obj.DeepCopyObject()
	return m.DeleteAllOfErr
}

func (m *mockClient) LastDeleteAllOf() runtime.Object {
	return m.lastDeleteAllOf
}

func (m *mockClient) Status() client.StatusWriter {
	return m.statusWriter
}

func (s *mockStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	s.lastStatusUpdate = obj.DeepCopyObject()
	if s.ErrChan != nil {
		err := <-s.ErrChan
		return err
	}
	return s.StatusUpdateErr

}

func (m *mockClient) LastStatusUpdate() runtime.Object {
	return m.lastStatusUpdate
}

func (s *mockStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	s.lastStatusPatch = obj.DeepCopyObject()
	return s.StatusPatchErr
}

func (m *mockClient) LastStatusPatch() runtime.Object {
	return m.lastStatusPatch
}
