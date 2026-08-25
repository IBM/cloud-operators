package controllers

import (
	"reflect"
	"testing"
	"unicode"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint: staticcheck
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestSetUpControllers(t *testing.T) {
	t.Run("real k8s manager succeeds", func(t *testing.T) {
		if testing.Short() {
			t.SkipNow()
		}
		_, err := SetUpControllers(k8sManager)
		require.NoError(t, err)
	})

	t.Run("all setups are called correctly", func(t *testing.T) {
		t.Parallel()
		mgr := &mockManager{T: t}

		var reconcilers []reconciler
		_, setupAllErr := setUpControllers(mgr, func(err *error, r reconciler, mgr manager.Manager, options controller.Options) {
			reconcilers = append(reconcilers, r)
		})
		assert.NoError(t, setupAllErr)
		require.Len(t, reconcilers, 3)
		assert.IsType(t, &BindingReconciler{}, reconcilers[0])
		assert.IsType(t, &ServiceReconciler{}, reconcilers[1])
		assert.IsType(t, &TokenReconciler{}, reconcilers[2])
	})

	t.Run("return setup error", func(t *testing.T) {
		t.Parallel()
		mgr := &mockManager{T: t}

		_, setupAllErr := setUpControllers(mgr, func(err *error, r reconciler, mgr manager.Manager, options controller.Options) {
			*err = errors.New("some error")
		})
		assert.EqualError(t, setupAllErr, "Unable to setup controller: some error")
	})
}

type mockReconciler struct {
	setup func(mgr ctrl.Manager, options controller.Options) error
}

func (m *mockReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return m.setup(mgr, options)
}

func TestSetupWithManagerOrErr(t *testing.T) {
	t.Parallel()

	t.Run("no error", func(t *testing.T) {
		someMgr := &mockManager{T: t}
		someOptions := controller.Options{MaxConcurrentReconciles: 1}
		r := &mockReconciler{
			setup: func(mgr manager.Manager, options controller.Options) error {
				assert.Equal(t, someMgr, mgr)
				assert.Equal(t, someOptions, options)
				return nil
			},
		}
		var err error

		setupWithManagerOrErr(&err, r, someMgr, someOptions)
		assert.NoError(t, err)
	})

	t.Run("new error", func(t *testing.T) {
		someMgr := &mockManager{T: t}
		someOptions := controller.Options{MaxConcurrentReconciles: 1}
		r := &mockReconciler{
			setup: func(mgr manager.Manager, options controller.Options) error {
				assert.Equal(t, someMgr, mgr)
				assert.Equal(t, someOptions, options)
				return errors.New("some error")
			},
		}
		var err error

		setupWithManagerOrErr(&err, r, someMgr, someOptions)
		assert.EqualError(t, err, "some error")
	})

	t.Run("existing error", func(t *testing.T) {
		someMgr := &mockManager{T: t}
		someOptions := controller.Options{MaxConcurrentReconciles: 1}
		r := &mockReconciler{
			setup: func(mgr manager.Manager, options controller.Options) error {
				panic("should not be called")
			},
		}
		err := errors.New("some error")

		setupWithManagerOrErr(&err, r, someMgr, someOptions)
		assert.EqualError(t, err, "some error")
	})
}

func TestSetUpControllerDependencies(t *testing.T) {
	t.Run("actual k8s manager dependencies", func(t *testing.T) {
		if testing.Short() {
			t.SkipNow()
		}
		c := setUpControllerDependencies(k8sManager)
		assertNoNilFields(t, c)
	})

	t.Run("fake manager for unit test dependencies", func(t *testing.T) {
		t.Parallel()
		c := setUpControllerDependencies(&mockManager{T: t})
		assertNoNilFields(t, c)
	})
}

func assertNoNilFields(t *testing.T, v interface{}) {
	assertNoNilFieldsReflect(t, reflect.ValueOf(v), "")
	if t.Failed() {
		t.Log("See the above failures for fields that must be filled in, inside controllers/manager_setup.go")
	}
}

func assertNoNilFieldsReflect(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	switch v.Kind() {
	case reflect.Ptr:
		assertNoNilFieldsReflect(t, v.Elem(), name)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			typeField := v.Type().Field(i)
			if len(typeField.Name) > 0 && unicode.IsUpper([]rune(typeField.Name)[0]) {
				assertNoNilFieldsReflect(t, field, joinFields(name, typeField.Name))
			}
		}
	default:
		if v.IsZero() {
			t.Errorf("Field %q is not set up properly for controllers", name)
		}
	}
}

func joinFields(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

type mockManager struct {
	ctrl.Manager
	T *testing.T
}

func (m *mockManager) GetClient() client.Client {
	return fake.NewFakeClient()
}

func (m *mockManager) GetScheme() *runtime.Scheme {
	return schemas(m.T)
}

func (m *mockManager) GetConfig() *rest.Config {
	return nil
}

func (m *mockManager) SetFields(interface{}) error {
	return nil
}

func (m *mockManager) GetCache() cache.Cache {
	return nil
}

func (m *mockManager) GetEventRecorderFor(string) record.EventRecorder {
	return nil
}

func (m *mockManager) Add(c manager.Runnable) error {
	panic("BELA")

// return c.(inject.Injector).InjectFunc(m.SetFields)
}
