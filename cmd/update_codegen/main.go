//go:generate go run .

// Command update_codegen updates generated code in this project.
//
// Currently enables code generation without a vendor directory or GOPATH by using Go Modules to detect file paths.
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

const thisModule = "github.com/ibm/cloud-operators"

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

	operatorsPath, err := getPackageFilePath(thisModule, "cmd/update_codegen")
	if err != nil {
		return err
	}

	packages := []string{
		"pkg/apis/...",
		"pkg/lib/resource/v1",
		"pkg/lib/keyvalue/v1",
	}
	for i := range packages {
		packages[i] = thisModule + "/" + packages[i]
	}

	cmd := exec.Command("go", "run", "k8s.io/code-generator/cmd/deepcopy-gen",
		"--go-header-file", filepath.Join(operatorsPath, "hack", "boilerplate.go.txt"),
		"--input-dirs", strings.Join(packages, ","),
		"--output-base", tmpDir,
		"--output-file-base", "zz_generated.deepcopy",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	fmt.Println("Moving generated files back into this repo's path...")
	generatedPathComps := append([]string{tmpDir}, strings.Split(thisModule, "/")...)
	generatedPath := filepath.Join(generatedPathComps...)
	return filepath.Walk(generatedPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		outputPath := filepath.Join(operatorsPath, strings.TrimPrefix(path, generatedPath+string(filepath.Separator)))
		if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
			return err
		}
		return os.Rename(path, outputPath)
	})
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