package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/blang/semver/v4"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

func main() {
	output := flag.String("output", "out", "Output directory for generated files")
	repoRoot := flag.String("root", ".", "Repo root directory")
	version := flag.String("version", "", "The release version to generate")
	flag.Parse()

	if err := run(*output, *repoRoot, *version); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

type Data struct {
	CRDs           []CRD
	DeploymentSpec appsv1.DeploymentSpec
	Examples       []runtime.RawExtension
	Image          string
	Name           string
	Now            string
	RBAC           []roleRules
	README         string
	Version        string
}

type roleRules struct {
	Rules []rbacv1.PolicyRule `json:"rules"`
}

func run(output, repoRoot, versionStr string) error {
	if versionStr == "" {
		return errors.New("Version flag must be set")
	}
	version, err := semver.Parse(versionStr)
	if err != nil {
		return errors.Wrap(err, "Invalid value for version")
	}

	if err := os.MkdirAll(output, 0700); err != nil {
		return err
	}

	templateDir := filepath.Join(repoRoot, "internal/cmd/genolm/templates")
	templates := template.New("")
	templates, err = templates.Funcs(map[string]interface{}{
		"base64":    encodeBase64,
		"include":   renderTemplateFile(templates),
		"indent":    indentLines,
		"json":      templateJSONMarshal,
		"trimSpace": strings.TrimSpace,
		"yaml":      templateYAMLMarshal,
	}).ParseGlob(filepath.Join(templateDir, "*"))
	if err != nil {
		return err
	}

	// README
	readme, err := os.Open(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		return err
	}
	defer readme.Close()

	// Examples
	var samples []runtime.RawExtension
	for _, name := range []string{"translator.yaml", "translator-binding.yaml"} {
		sample, err := ioutil.ReadFile(filepath.Join(repoRoot, "config/samples", name))
		if err != nil {
			return err
		}
		var raw runtime.RawExtension
		err = yaml.Unmarshal(sample, &raw)
		if err != nil {
			return err
		}
		samples = append(samples, raw)
	}

	// DeploymentSpec
	var deployment appsv1.Deployment
	deploymentBytes, err := ioutil.ReadFile(filepath.Join(repoRoot, "out/apps_v1_deployment_ibmcloud-operator-controller-manager.yaml"))
	if err != nil {
		return errors.Wrap(err, "Error reading generated deployment file. Did kustomize run yet?")
	}
	err = yaml.Unmarshal(deploymentBytes, &deployment)
	if err != nil {
		return err
	}
	deploymentSpec := deployment.Spec

	// RBAC
	var rbac roleRules
	rbacFiles, err := filepath.Glob(filepath.Join(output, "rbac.*.yaml"))
	if err != nil {
		return err
	}
	for _, path := range rbacFiles {
		buf, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var role rbacv1.Role
		err = yaml.Unmarshal(buf, &role)
		if err != nil {
			return err
		}
		kind := role.GetObjectKind().GroupVersionKind().Kind
		if kind == "ClusterRole" || kind == "Role" {
			rbac.Rules = append(rbac.Rules, role.Rules...)
		}
	}

	// CRDs
	var crds []CRD
	{
		var bindingCRD apiextensionsv1beta1.CustomResourceDefinition
		bindingCRDBytes, err := ioutil.ReadFile(filepath.Join(repoRoot, "out/apiextensions.k8s.io_v1beta1_customresourcedefinition_bindings.ibmcloud.ibm.com.yaml"))
		if err != nil {
			return errors.Wrap(err, "Error reading generated CRD file. Did kustomize run yet?")
		}
		err = yaml.Unmarshal(bindingCRDBytes, &bindingCRD)
		if err != nil {
			return err
		}
		crds = append(crds, NewCRD(
			bindingCRD,
			[]TypeMeta{
				{Kind: "Secret", Name: "", Version: "v1"},
				{Kind: "ConfigMap", Name: "", Version: "v1"},
				{Kind: "Binding", Name: "", Version: "v1beta1"},
				{Kind: "Service", Name: "", Version: "v1beta1"},
			},
			map[string][]string{
				"secretName": {"urn:alm:descriptor:text", "urn:alm:descriptor:io.kubernetes:Secret", "binding:env:object:secret"},
			},
		)) // TODO
	}
	{
		var serviceCRD apiextensionsv1beta1.CustomResourceDefinition
		serviceCRDBytes, err := ioutil.ReadFile(filepath.Join(repoRoot, "out/apiextensions.k8s.io_v1beta1_customresourcedefinition_services.ibmcloud.ibm.com.yaml"))
		if err != nil {
			return errors.Wrap(err, "Error reading generated CRD file. Did kustomize run yet?")
		}
		err = yaml.Unmarshal(serviceCRDBytes, &serviceCRD)
		if err != nil {
			return err
		}
		crds = append(crds, NewCRD(
			serviceCRD,
			[]TypeMeta{
				{Kind: "Secret", Name: "", Version: "v1"},
				{Kind: "ConfigMap", Name: "", Version: "v1"},
				{Kind: "Binding", Name: "", Version: "v1beta1"},
				{Kind: "Service", Name: "", Version: "v1beta1"},
			},
			map[string][]string{
				"secretName": {"urn:alm:descriptor:text", "urn:alm:descriptor:io.kubernetes:Secret", "binding:env:object:secret"},
			},
		)) // TODO
	}

	data := Data{
		CRDs:           crds,
		DeploymentSpec: deploymentSpec,
		Examples:       samples,
		Image:          "cloudoperators/ibmcloud-operator",
		Name:           "ibmcloud-operator",
		Now:            time.Now().UTC().Format(time.RFC3339),
		RBAC:           []roleRules{rbac},
		README:         prepREADME(readme),
		Version:        version.String(),
	}

	for inPath, outPath := range map[string]string{
		"package.yaml":               "ibmcloud-operator.package.yaml",
		"clusterserviceversion.yaml": fmt.Sprintf("ibmcloud_operator.v%s.clusterserviceversion.yaml", data.Version),
	} {
		err := writeFile(filepath.Join(output, outPath), templates.Lookup(inPath), data)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, tmpl *template.Template, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func renderTemplateFile(templates *template.Template) func(name string, data interface{}) ([]byte, error) {
	return func(name string, data interface{}) ([]byte, error) {
		var buf bytes.Buffer
		err := templates.ExecuteTemplate(&buf, name, data)
		return buf.Bytes(), err
	}
}

func prepREADME(readme io.Reader) string {
	scanner := bufio.NewScanner(readme)
	include := false
	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if include {
			if line != "" || buf.Len() > 0 { // skip leading blank lines
				buf.WriteString(line)
				buf.WriteRune('\n')
			}
		} else if strings.HasPrefix(line, "# ") {
			// skip up to and including main header lines
			include = true
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	return buf.String()
}

func indentLines(spaces int, s string) string {
	indent := fmt.Sprintf(fmt.Sprintf("%%%ds", spaces), "")

	r := strings.NewReader(s)
	scanner := bufio.NewScanner(r)
	var buf bytes.Buffer
	for scanner.Scan() {
		buf.WriteString(indent)
		buf.WriteString(scanner.Text())
		buf.WriteRune('\n')
	}
	return buf.String()
}

func templateJSONMarshal(v interface{}) (string, error) {
	buf, err := json.Marshal(v)
	return string(buf), err
}

func templateYAMLMarshal(v interface{}) (string, error) {
	buf, err := yaml.Marshal(v)
	return string(buf), err
}
