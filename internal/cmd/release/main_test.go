package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		description string
		version     string
		csvFile     bool
		packageFile bool
		expectErr   string
	}{
		{
			description: "no CSV file",
			version:     "v1.2.3",
			csvFile:     false,
			packageFile: true,
			expectErr:   "failed to read cluster service version file: open : no such file or directory",
		},
		{
			description: "no package file",
			version:     "v1.2.3",
			csvFile:     true,
			packageFile: false,
			expectErr:   "failed to read package file: open : no such file or directory",
		},
		{
			description: "no version",
			version:     "",
			csvFile:     true,
			packageFile: true,
			expectErr:   "version is required",
		},
		{
			description: "happy path",
			version:     "v1.2.3",
			csvFile:     true,
			packageFile: true,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			dir := t.TempDir()
			args := Args{
				Version: tc.version,
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
				assert.Equal(t, "github.com", r.URL.Host)
				switch r.Method {
				case http.MethodGet:
					switch r.URL.Path {
					case "/repos/k8s-operatorhub/community-operators/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml":
						fmt.Fprintf(w, `{"sha": %q}`, k8sPackageSHA)
					case "/repos/redhat-openshift-ecosystem/community-operators-prod/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml":
						fmt.Fprintf(w, `{"sha": %q}`, openshiftPackageSHA)
					default:
						t.Fatal("Unrecognized github.com GET path:", r.URL.Path)
					}
				case http.MethodPut:
					body, err := ioutil.ReadAll(r.Body)
					require.NoError(t, err)
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
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
