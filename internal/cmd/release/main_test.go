package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	const (
		someFork       = "some fork"
		someVersion    = "v1.2.3"
		existingK8sPR  = "https://github.com/org/repo/pull/123"
		newOpenShiftPR = "https://github.com/org/repo/pull/456"
	)
	for _, tc := range []struct {
		description string
		version     string
		forkOrg     string
		csvFile     bool
		packageFile bool
		expectOut   string
		expectErr   string
	}{
		{
			description: "no CSV file",
			version:     someVersion,
			forkOrg:     someFork,
			csvFile:     false,
			packageFile: true,
			expectErr:   "failed to read cluster service version file: open : no such file or directory",
		},
		{
			description: "no package file",
			version:     someVersion,
			forkOrg:     someFork,
			csvFile:     true,
			packageFile: false,
			expectErr:   "failed to read package file: open : no such file or directory",
		},
		{
			description: "no version",
			version:     "",
			forkOrg:     someFork,
			csvFile:     true,
			packageFile: true,
			expectErr:   "version is required",
		},
		{
			description: "no fork org",
			version:     someVersion,
			forkOrg:     "",
			csvFile:     true,
			packageFile: true,
			expectErr:   "fork org is required",
		},
		{
			description: "happy path",
			version:     someVersion,
			forkOrg:     someFork,
			csvFile:     true,
			packageFile: true,
			expectOut: fmt.Sprintf(`
Kubernetes PR opened: %s
OpenShift PR opened: %s
`, existingK8sPR, newOpenShiftPR),
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			dir := t.TempDir()
			var output bytes.Buffer
			args := Args{
				GitHubToken: "some token",
				Version:     tc.version,
				ForkOrg:     tc.forkOrg,
				Output:      &output,
			}
			if tc.csvFile {
				args.CSVFile = filepath.Join(dir, "csv.yaml")
				require.NoError(t, ioutil.WriteFile(args.CSVFile, []byte(`CSV contents`), 0600))
			}
			if tc.packageFile {
				args.PackageFile = filepath.Join(dir, "package.yaml")
				require.NoError(t, ioutil.WriteFile(args.PackageFile, []byte(`CSV contents`), 0600))
			}
			const (
				k8sPackageSHA       = "abc123"
				openshiftPackageSHA = "def456"
			)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "https", r.URL.Scheme)
				assert.Equal(t, "api.github.com", r.URL.Host)
				if r.Body == nil {
					r.Body = ioutil.NopCloser(bytes.NewReader(nil))
				}
				body, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)
				switch r.Method {
				case http.MethodGet:
					switch r.URL.Path {
					case "/repos/k8s-operatorhub/community-operators/pulls":
						query := url.Values{
							"head": []string{someFork + ":release-" + someVersion + "-"},
						}
						assert.True(t, strings.HasPrefix(r.URL.RawQuery, query.Encode()), "Query = %s", r.URL.RawQuery)
						fmt.Fprintf(w, `[
							{"html_url": %q}
						]`, existingK8sPR)
					case "/repos/redhat-openshift-ecosystem/community-operators-prod/pulls":
						fmt.Fprint(w, `[]`)
					case "/repos/k8s-operatorhub/community-operators/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml":
						fmt.Fprintf(w, `{"sha": %q}`, k8sPackageSHA)
					case "/repos/redhat-openshift-ecosystem/community-operators-prod/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml":
						fmt.Fprintf(w, `{"sha": %q}`, openshiftPackageSHA)
					default:
						t.Fatal("Unrecognized github.com GET path:", r.URL.Path)
					}
				case http.MethodPut:
					switch r.URL.Path {
					case "/repos/k8s-operatorhub/community-operators/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml":
						assert.Contains(t, string(body), fmt.Sprintf(`"sha":%q`, k8sPackageSHA))
						fmt.Fprint(w, `{}`)
					case "/repos/redhat-openshift-ecosystem/community-operators-prod/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml":
						assert.Contains(t, string(body), fmt.Sprintf(`"sha":%q`, openshiftPackageSHA))
						fmt.Fprint(w, `{}`)
					case "/repos/k8s-operatorhub/community-operators/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml",
						"/repos/redhat-openshift-ecosystem/community-operators-prod/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml":
						fmt.Fprint(w, `{}`)
					default:
						t.Fatal("Unrecognized github.com PUT path:", r.URL.Path)
					}
				case http.MethodPost:
					switch r.URL.Path {
					case "/repos/redhat-openshift-ecosystem/community-operators-prod/pulls":
						bodyStr := string(body)
						assert.Contains(t, bodyStr, `"head":"`+someFork+`:release-`+someVersion+`-`)
						assert.Contains(t, bodyStr, `"base":"main"`)
						fmt.Fprintf(w, `{"html_url":%q}`, newOpenShiftPR)
					default:
						t.Fatal("Unrecognized github.com POST path:", r.URL.Path)
					}
				default:
					t.Fatal("Unrecognized github.com HTTP method:", r.Method, r.URL.Path)
				}
			})
			deps := Deps{
				GitHub: &GitHub{doRequest: func(req *http.Request) (*http.Response, error) {
					recorder := httptest.NewRecorder()
					handler.ServeHTTP(recorder, req)
					return recorder.Result(), nil
				}},
			}
			err := run(args, deps)
			assert.Equal(t, strings.TrimPrefix(tc.expectOut, "\n"), output.String())
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
