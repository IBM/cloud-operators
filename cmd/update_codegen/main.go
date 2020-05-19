//go:generate go run .

// Command update_codegen runs several scripts from k8s.io/code-generator to update generated code in this project.
// Currently enables code generation without a vendor directory or GOPATH by using Go Modules instead.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

const thisPackage = "github.com/ibm/cloud-operators"

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("Updating generated code...")
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	operatorsPath, err := getPackageFilePath(thisPackage, "cmd/update_codegen")
	if err != nil {
		return err
	}

	codeGeneratorPath, err := getPackageFilePath("k8s.io/code-generator", "pkg/util")
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "run", "k8s.io/code-generator/cmd/deepcopy-gen",
		"-O", "zz_generated.deepcopy",
		"-i", thisPackage+"/pkg/apis/...",
		"--output-base", tmpDir,
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
		thisPackage+"/pkg/lib/resource/v1",
		thisPackage+"/pkg/lib",
		"resource:v1",
		"--output-base", tmpDir,
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
		thisPackage+"/pkg/lib/keyvalue/v1",
		thisPackage+"/pkg/lib",
		"keyvalue:v1",
		"--output-base", tmpDir,
		"--go-header-file", filepath.Join(operatorsPath, "hack", "boilerplate.go.txt"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	fmt.Println("Moving generated files back into this repo's path...")
	generatedPathComps := append([]string{tmpDir}, strings.Split(thisPackage, "/")...)
	generatedPath := filepath.Join(generatedPathComps...)
	filepath.Walk(generatedPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		outputPath := filepath.Join(operatorsPath, strings.TrimPrefix(path, generatedPath+string(filepath.Separator)))
		if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
			return err
		}
		return os.Rename(path, outputPath)
	})
	return nil
}

// getPackageFilePath returns the file path to the main package, using goFileSubPkg as a means to load its file paths
// 'goFileSubPkg' is required for loading packages without Go files at their root
func getPackageFilePath(mainPkg, goFileSubPkg string) (string, error) {
	path, err := getSubpackageFilePath(path.Join(mainPkg, goFileSubPkg))
	if err != nil {
		return "", err
	}
	subPathComponents := strings.Split(goFileSubPkg, "/")
	for range subPathComponents {
		path = filepath.Join(path, "..")
	}
	return path, nil // trim off subpackage file path
}

// getSubpackageFilePath loads the file path for the given package using Go Modules. Requires Go files exist in the package.
// Returns the project's file path for the current module package, and a Go Modules path for all other packages.
func getSubpackageFilePath(packageName string) (string, error) {
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
