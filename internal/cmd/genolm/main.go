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
	DeploymentSpec appsv1.DeploymentSpec
	Examples       []json.RawMessage
	Image          string
	Name           string
	Now            string
	README         string
	Version        string
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
	var samples []json.RawMessage
	for _, name := range []string{"translator.yaml", "translator-binding.yaml"} {
		sample, err := ioutil.ReadFile(filepath.Join(repoRoot, "config/samples", name))
		if err != nil {
			return err
		}
		sample, err = yaml.YAMLToJSON(sample)
		if err != nil {
			return err
		}
		samples = append(samples, sample)
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

	data := Data{
		DeploymentSpec: deploymentSpec,
		Examples:       samples,
		Image:          "cloudoperators/ibmcloud-operator",
		Name:           "ibmcloud-operator",
		Now:            time.Now().UTC().Format(time.RFC3339),
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
