package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v2"
)

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return mutateYaml(os.Stdin, os.Stdout)
	}

	for _, arg := range args {
		f, err := os.Open(arg)
		if err != nil {
			return err
		}
		defer f.Close()
		var buf bytes.Buffer
		err = mutateYaml(f, &buf)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(arg, buf.Bytes(), 0644) // nolint:gosec // mimics original file permissions from kubebuilder
		if err != nil {
			return err
		}
	}
	return nil
}

func mutateYaml(r io.Reader, w io.Writer) error {
	var yamlData yaml.MapSlice
	err := yaml.NewDecoder(r).Decode(&yamlData)
	if err != nil {
		return err
	}

	removeValueType(&yamlData, false)
	versions := getVersions(yamlData)
	version := latestVersion(versions)
	setVersion(&yamlData, version, 0)
	return yaml.NewEncoder(w).Encode(yamlData)
}

func removeValueType(v interface{}, inValue bool) (remove bool) {
	d, set := ptrSetter(v)
	switch d := d.(type) {
	case []interface{}:
		newSlice := make([]interface{}, 0, len(d))
		for ix := range d {
			item := d[ix]
			shouldRemove := removeValueType(&item, inValue)
			if !shouldRemove {
				newSlice = append(newSlice, item)
			}
		}
		set(newSlice)
		return false
	case yaml.MapSlice:
		newSlice := make(yaml.MapSlice, 0, len(d))
		for ix := range d {
			item := d[ix]
			shouldRemove := removeValueType(&item, inValue)
			if !shouldRemove {
				newSlice = append(newSlice, item)
			}
		}
		set(newSlice)
		return false
	case yaml.MapItem:
		if inValue && d.Key == "type" && d.Value == "object" {
			return true
		}
		b := removeValueType(&d.Value, inValue || d.Key == "value")
		set(d)
		return b
	default:
		return false
	}
}

func ptrSetter(v interface{}) (value interface{}, setPtr func(interface{})) {
	val := reflect.ValueOf(v).Elem()
	return val.Interface(), func(newValue interface{}) {
		val.Set(reflect.ValueOf(newValue))
	}
}

func setVersion(v interface{}, version string, depth int) {
	const maxDepth = 3
	if depth > maxDepth {
		return
	}
	depth++

	d, set := ptrSetter(v)
	switch d := d.(type) {
	case []interface{}:
		newSlice := make([]interface{}, 0, len(d))
		for ix := range d {
			item := d[ix]
			setVersion(&item, version, depth)
			newSlice = append(newSlice, item)
		}
		set(newSlice)
	case yaml.MapSlice:
		newSlice := make(yaml.MapSlice, 0, len(d))
		for ix := range d {
			item := d[ix]
			setVersion(&item, version, depth)
			newSlice = append(newSlice, item)
		}
		set(newSlice)
	case yaml.MapItem:
		setVersion(&d.Value, version, depth)
		if d.Key == "version" {
			d.Value = version
		}
		set(d)
	}
}

func getVersions(v interface{}) (versions []string) {
	return recursiveGetVersions(v, false)
}

func recursiveGetVersions(v interface{}, foundVersions bool) (versions []string) {
	switch d := v.(type) {
	case []interface{}:
		for ix := range d {
			item := d[ix]
			versions = append(versions, recursiveGetVersions(item, foundVersions)...)
		}
	case yaml.MapSlice:
		for ix := range d {
			item := d[ix]
			versions = append(versions, recursiveGetVersions(item, foundVersions)...)
		}
	case yaml.MapItem:
		if d.Key == "versions" {
			return recursiveGetVersions(d.Value, true)
		}
		if !foundVersions {
			return append(versions, recursiveGetVersions(d.Value, foundVersions)...)
		}
		if d.Key != "name" {
			return
		}
		return append(versions, d.Value.(string))
	}
	return
}

func latestVersion(versions []string) string {
	// sort versions lowest to highest. i.e. v1alpha1, v1beta1, v1beta2, v1
	sort.Slice(versions, func(a, b int) bool {
		strA := versions[a]
		strB := versions[b]
		// artificially sort 'v1' as greater than 'v1alpha1'. this works because '~' > 'a-zA-Z'
		if isDigits(strings.TrimPrefix(strA, "v")) {
			strA += "~"
		}
		if isDigits(strings.TrimPrefix(strB, "v")) {
			strA += "~"
		}
		return strA < strB
	})
	return versions[len(versions)-1]
}

func isDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
