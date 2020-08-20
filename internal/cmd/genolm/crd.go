package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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

func NewCRD(src apiextensionsv1beta1.CustomResourceDefinition, ownedResources []TypeMeta, xDescriptors map[string][]string) CRD {
	if len(src.Spec.Versions) == 0 {
		panic(fmt.Sprintf("Versions section is empty: %#v", src.Spec))
	}

	latestVersion := src.Spec.Versions[len(src.Spec.Versions)-1]
	return CRD{
		DisplayName:       src.Spec.Names.Kind,
		Description:       latestVersion.Schema.OpenAPIV3Schema.Description,
		Group:             src.Spec.Group,
		Kind:              src.Spec.Names.Kind,
		Name:              src.GetObjectMeta().GetName(),
		OwnedResources:    ownedResources,
		SpecDescriptors:   descriptorsFor("spec", latestVersion, xDescriptors),
		StatusDescriptors: descriptorsFor("status", latestVersion, xDescriptors),
	}
}

func descriptorsFor(kind string, version apiextensionsv1beta1.CustomResourceDefinitionVersion, xDescriptors map[string][]string) []Descriptor {
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
