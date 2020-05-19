package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("Updating generated code...")
	operatorsPath, err := getPackageFilePath("github.com/ibm/cloud-operators/cmd/update_codegen")
	if err != nil {
		return err
	}
	operatorsPath = filepath.Join(operatorsPath, "..", "..") // trim off cmd/update_codegen

	codeGeneratorPath, err := getPackageFilePath("k8s.io/code-generator/pkg/util")
	if err != nil {
		return err
	}
	codeGeneratorPath = filepath.Join(codeGeneratorPath, "..", "..") // trim off pkg/util

	cmd := exec.Command("bash", filepath.Join(codeGeneratorPath, "generate-groups.sh"),
		"deepcopy",
		"github.com/ibm/cloud-operators/pkg/lib/resource/v1",
		"github.com/ibm/cloud-operators/pkg/lib",
		"resource:v1",
		"--output-base", operatorsPath,
		"--go-header-file", filepath.Join(operatorsPath, "hack", "boilerplate.go.txt"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("bash", filepath.Join(codeGeneratorPath, "generate-groups.sh"),
		"deepcopy",
		"github.com/ibm/cloud-operators/pkg/lib/keyvalue/v1",
		"github.com/ibm/cloud-operators/pkg/lib",
		"keyvalue:v1",
		"--go-header-file", filepath.Join(operatorsPath, "hack", "boilerplate.go.txt"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func getPackageFilePath(packageName string) (string, error) {
	cfg := &packages.Config{Mode: packages.NeedFiles}
	pkgs, err := packages.Load(cfg, packageName)
	if err != nil {
		return "", errors.Wrapf(err, "Error loading packages for '%s'", packageName)
	}
	if len(pkgs) == 0 {
		return "", errors.Errorf("Package not found: '%s'", packageName)
	}
	allFiles := append(pkgs[0].GoFiles, pkgs[0].OtherFiles...)
	if len(allFiles) == 0 {
		return "", errors.Errorf("No files found inside package. Therefore no static files could be loaded for '%s'", packageName)
	}
	return path.Dir(allFiles[0]), nil
}
