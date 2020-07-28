package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"

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
		err = ioutil.WriteFile(arg, buf.Bytes(), 0644)
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
	return yaml.NewEncoder(w).Encode(yamlData)
}

func removeValueType(v interface{}, inValue bool) (remove bool) {
	d, set := ptrSetter(v)
	switch d := d.(type) {
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
