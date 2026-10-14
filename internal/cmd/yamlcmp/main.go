package main

import (
	"fmt"
	"log"
	"os"
	"reflect"

	"gopkg.in/yaml.v2"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.SetFlags(0)
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: yamlcmp FILE FILE")
	}

	left, err := readYAML(args[0])
	if err != nil {
		return err
	}
	right, err := readYAML(args[1])
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(left, right) {
		return fmt.Errorf("%s and %s contain different YAML values", args[0], args[1])
	}
	return nil
}

func readYAML(filename string) (interface{}, error) {
	contents, err := os.ReadFile(filename) // #nosec G304 -- filenames are command-line arguments
	if err != nil {
		return nil, err
	}

	var value interface{}
	if err := yaml.UnmarshalStrict(contents, &value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	return value, nil
}
