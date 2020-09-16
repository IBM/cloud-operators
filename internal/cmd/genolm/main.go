package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/blang/semver/v4"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	rbacv1 "k8s.io/api/rbac/v1"
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
	Deployments    []Deployment
	Examples       []runtime.RawExtension
	Image          string
	Maintainers    []Maintainer
	Name           string
	Now            string
	RBAC           []roleRules
	README         string
	ReplaceVersion string
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

	replaceVersion, err := getReplaceVersion(repoRoot, version)
	if err != nil {
		return err
	}

	readmeFile, err := os.Open(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		return err
	}
	defer readmeFile.Close()
	readme := prepREADME(readmeFile)

	samples, err := getSamples(repoRoot)
	if err != nil {
		return err
	}

	deployments, err := getDeployments(output)
	if err != nil {
		return err
	}

	rbac, err := getRBAC(output)
	if err != nil {
		return err
	}

	crds, err := getCRDs(repoRoot)
	if err != nil {
		return err
	}

	maintainers, err := getMaintainers(repoRoot)
	if err != nil {
		return err
	}

	data := Data{
		CRDs:           crds,
		Deployments:    deployments,
		Examples:       samples,
		Image:          "cloudoperators/ibmcloud-operator",
		Maintainers:    maintainers,
		Name:           "ibmcloud-operator",
		Now:            time.Now().UTC().Format(time.RFC3339),
		RBAC:           []roleRules{rbac},
		README:         readme,
		ReplaceVersion: replaceVersion,
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

func getRBAC(output string) (roleRules, error) {
	var rbac roleRules
	rbacFiles, err := filepath.Glob(filepath.Join(output, "rbac.*.yaml"))
	if err != nil {
		return roleRules{}, err
	}
	for _, path := range rbacFiles {
		buf, err := ioutil.ReadFile(path)
		if err != nil {
			return roleRules{}, err
		}
		var role rbacv1.Role
		err = yaml.Unmarshal(buf, &role)
		if err != nil {
			return roleRules{}, err
		}
		kind := role.GetObjectKind().GroupVersionKind().Kind
		if kind == "ClusterRole" || kind == "Role" {
			rbac.Rules = append(rbac.Rules, role.Rules...)
		}
	}
	return rbac, nil
}

func getSamples(repoRoot string) ([]runtime.RawExtension, error) {
	var samples []runtime.RawExtension
	for _, name := range []string{"translator.yaml", "translator-binding.yaml"} {
		sample, err := ioutil.ReadFile(filepath.Join(repoRoot, "config/samples", name))
		if err != nil {
			return nil, err
		}
		var raw runtime.RawExtension
		err = yaml.Unmarshal(sample, &raw)
		if err != nil {
			return nil, err
		}
		samples = append(samples, raw)
	}
	return samples, nil
}
