package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type CRD struct {
	DisplayName       string       `json:"displayName"`
	Description       string       `json:"description"`
	Group             string       `json:"group"`
	Kind              string       `json:"kind"`
	Name              string       `json:"name"`
	OwnedResources    []TypeMeta   `json:"resources,omitempty"` // resources monitored & reconciled by this CRD
	SpecDescriptors   []Descriptor `json:"specDescriptors,omitempty"`
	StatusDescriptors []Descriptor `json:"statusDescriptors,omitempty"`
	ActionDescriptors []Descriptor `json:"actionDescriptors,omitempty"`
	Version           string       `json:"version"`
}

type TypeMeta struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Descriptor struct {
	DisplayName  string   `json:"displayName"`
	Description  string   `json:"description"`
	Path         string   `json:"path"`
	XDescriptors []string `json:"x-descriptors,omitempty"`
}

func getCRDs(repoRoot string) ([]CRD, error) {
	var crds []CRD
	var bindingCRD apiextensionsv1.CustomResourceDefinition
	bindingCRDBytes, err := ioutil.ReadFile(filepath.Join(repoRoot, "out/apiextensions.k8s.io_v1_customresourcedefinition_bindings.ibmcloud.ibm.com.yaml")) // #nosec G304 repoRoot comes from user input
	if err != nil {
		return nil, errors.Wrap(err, "Error reading generated CRD file. Did kustomize run yet?")
	}
	err = yaml.Unmarshal(bindingCRDBytes, &bindingCRD)
	if err != nil {
		return nil, err
	}
	crds = append(crds, NewCRD(
		bindingCRD,
		[]TypeMeta{
			{Kind: "Secret", Name: "", Version: "v1"},
			{Kind: "ConfigMap", Name: "", Version: "v1"},
			{Kind: "Binding", Name: "", Version: "v1"},
			{Kind: "Service", Name: "", Version: "v1"},
		},
		map[string][]string{
			"secretName": {"urn:alm:descriptor:text", "urn:alm:descriptor:io.kubernetes:Secret", "binding:env:object:secret"},
		},
	))

	var serviceCRD apiextensionsv1.CustomResourceDefinition
	serviceCRDBytes, err := ioutil.ReadFile(filepath.Join(repoRoot, "out/apiextensions.k8s.io_v1_customresourcedefinition_services.ibmcloud.ibm.com.yaml")) // #nosec G304 repoRoot comes from user input
	if err != nil {
		return nil, errors.Wrap(err, "Error reading generated CRD file. Did kustomize run yet?")
	}
	err = yaml.Unmarshal(serviceCRDBytes, &serviceCRD)
	if err != nil {
		return nil, err
	}
	crds = append(crds, NewCRD(
		serviceCRD,
		[]TypeMeta{
			{Kind: "Secret", Name: "", Version: "v1"},
			{Kind: "ConfigMap", Name: "", Version: "v1"},
			{Kind: "Binding", Name: "", Version: "v1"},
			{Kind: "Service", Name: "", Version: "v1"},
		},
		map[string][]string{
			"secretName": {"urn:alm:descriptor:text", "urn:alm:descriptor:io.kubernetes:Secret", "binding:env:object:secret"},
		},
	))
	return crds, nil
}

func NewCRD(src apiextensionsv1.CustomResourceDefinition, ownedResources []TypeMeta, xDescriptors map[string][]string) CRD {
	if len(src.Spec.Versions) == 0 {
		panic(fmt.Sprintf("Versions section is empty: %#v", src.Spec))
	}

	latestVersion := src.Spec.Versions[0]
	return CRD{
		DisplayName:       src.Spec.Names.Kind,
		Description:       latestVersion.Schema.OpenAPIV3Schema.Description,
		Group:             src.Spec.Group,
		Kind:              src.Spec.Names.Kind,
		Name:              src.GetObjectMeta().GetName(),
		OwnedResources:    ownedResources,
		SpecDescriptors:   descriptorsFor("spec", latestVersion, xDescriptors),
		StatusDescriptors: descriptorsFor("status", latestVersion, xDescriptors),
		Version:           latestVersion.Name,
	}
}

func descriptorsFor(kind string, version apiextensionsv1.CustomResourceDefinitionVersion, xDescriptors map[string][]string) []Descriptor {
	props := version.Schema.OpenAPIV3Schema.Properties[kind].Properties
	defaultXDescriptor := []string{"urn:alm:descriptor:text"}

	var descriptors []Descriptor
	for key, dict := range props {
		xDescriptor := defaultXDescriptor
		if desc, ok := xDescriptors[key]; ok {
			xDescriptor = desc
		}
		firstWord := getFirstWord(dict.Description)
		titleName := key
		description := dict.Description
		if firstWord != "" {
			// replace first word (assumed to be type name)
			titleName = titleCaseSpaced(firstWord)
			description = titleName + strings.TrimPrefix(dict.Description, firstWord)
		}

		descriptors = append(descriptors, Descriptor{
			DisplayName:  titleName,
			Description:  description,
			Path:         key,
			XDescriptors: xDescriptor,
		})
	}
	sort.Slice(descriptors, func(a, b int) bool {
		return descriptors[a].DisplayName < descriptors[b].DisplayName
	})
	return descriptors
}

func getFirstWord(sentence string) string {
	spaceIx := strings.IndexRune(sentence, ' ')
	if spaceIx == -1 {
		return ""
	}
	return sentence[:spaceIx]
}

func titleCaseSpaced(s string) string {
	var b strings.Builder
	runes := []rune(s)
	prevWasUpper := false
	for ix, c := range runes {
		switch {
		case ix == 0:
			b.WriteRune(unicode.ToTitle(c))
		case unicode.IsUpper(c) && !prevWasUpper:
			b.WriteRune(' ')
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
		prevWasUpper = unicode.IsUpper(c)
	}
	return b.String()
}
