package controllers

import (
	"reflect"
	"testing"
	"unicode"
)

func TestSetUpControllers(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	c := setUpControllerDependencies(k8sManager)

	assertNoNilFields(t, c)
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
