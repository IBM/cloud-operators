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
		someFork      = "some-fork"
		someToken     = "abc123"
		someVersion   = "v1.2.3"
		someUserName  = "Joe Schmoe"
		someUserEmail = "joe@example.com"
		someCRDGlob   = "apiextensions.k8s.io_*_customresourcedefinition_*.ibmcloud.ibm.com.yaml"
	)
	dir := t.TempDir()
	someCSVFile := filepath.Join(dir, "ibmcloud-operator.package.yaml")
	require.NoError(t, ioutil.WriteFile(someCSVFile, []byte(`CSV contents`), 0600))
	somePackageFile := filepath.Join(dir, "ibmcloud_operator.v1.2.3.clusterserviceversion.yaml")
	require.NoError(t, ioutil.WriteFile(somePackageFile, []byte(`package contents`), 0600))
	someCRDFile := filepath.Join(dir, "apiextensions.k8s.io_v1beta1_customresourcedefinition_mycrd.ibmcloud.ibm.com.yaml")
	require.NoError(t, ioutil.WriteFile(someCRDFile, []byte(`CRD contents`), 0600))

	const (
		existingK8sPR  = "https://github.com/org/repo/pull/123"
		newOpenShiftPR = "https://github.com/org/repo/pull/456"
	)
	for _, tc := range []struct {
		description string
		args        []string
		expectOut   string
		expectErr   string
	}{
		{
			description: "missing flags",
			args:        []string{},
			expectErr: `Missing required flags:
    -crd-glob
    -csv
    -fork-org
    -gh-token
    -package
    -signoff-email
    -signoff-name
    -version`,
		},
		{
			description: "invalid flag type",
			args:        []string{"-draft=no"},
			expectErr:   `invalid boolean value "no" for -draft: parse error`,
		},
		{
			description: "happy path",
			args: []string{
				"-crd-glob", someCRDGlob,
				"-csv", someCSVFile,
				"-fork-org", someFork,
				"-gh-token", someToken,
				"-package", somePackageFile,
				"-signoff-email", someUserEmail,
				"-signoff-name", someUserName,
				"-version", someVersion,
			},
			expectOut: fmt.Sprintf(`
Kubernetes PR opened: %s
OpenShift PR opened: %s
`, existingK8sPR, newOpenShiftPR),
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			args, err := parseArgs(tc.args, ioutil.Discard)
			if err != nil {
				if tc.expectErr != "" {
					assert.EqualError(t, err, tc.expectErr)
					return
				}
				require.NoError(t, err)
			}

			startSHA := 0
			newSHA := func() string {
				startSHA++
				return fmt.Sprintf("%06d", startSHA)
			}
			var (
				k8sMainSHA          = newSHA()
				k8sPackageSHA       = newSHA()
				k8sCSVSHA           = newSHA()
				openshiftMainSHA    = newSHA()
				openshiftPackageSHA = newSHA()
				openshiftCSVSHA     = newSHA()
			)
			const (
				k8sRepo     = "/repos/k8s-operatorhub/community-operators"
				k8sForkRepo = "/repos/some-fork/community-operators"
				ocRepo      = "/repos/redhat-openshift-ecosystem/community-operators-prod"
				ocForkRepo  = "/repos/some-fork/community-operators-prod"
			)
			handler := newMockServerHandler(t, []mockRequest{
				// k8s
				{http.MethodGet, k8sRepo + "/git/ref/heads/main"},                                                                                 // get default branch from upstream
				{http.MethodPatch, k8sForkRepo + "/git/refs/heads/main"},                                                                          // force update fork default branch to same commit
				{http.MethodGet, k8sForkRepo + "/git/ref/heads/release-v1.2.3"},                                                                   // get release branch info
				{http.MethodPatch, k8sForkRepo + "/git/refs/heads/release-v1.2.3"},                                                                // branch exists, force update to same commit
				{http.MethodGet, k8sForkRepo + "/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}, // get old CSV hash
				{http.MethodPut, k8sForkRepo + "/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}, // update CSV
				{http.MethodGet, k8sForkRepo + "/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"},                            // get old package hash
				{http.MethodPut, k8sForkRepo + "/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"},                            // update package

				// openshift
				{http.MethodGet, ocRepo + "/git/ref/heads/main"},                                                                                 // get default branch from upstream
				{http.MethodPatch, ocForkRepo + "/git/refs/heads/main"},                                                                          // force update fork default branch to same commit
				{http.MethodGet, ocForkRepo + "/git/ref/heads/release-v1.2.3"},                                                                   // get release branch info
				{http.MethodPost, ocForkRepo + "/git/refs"},                                                                                      // branch does not exist, create new one at same commit
				{http.MethodGet, ocForkRepo + "/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}, // get old CSV hash
				{http.MethodPut, ocForkRepo + "/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}, // update CSV
				{http.MethodGet, ocForkRepo + "/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"},                            // get old package hash
				{http.MethodPut, ocForkRepo + "/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"},                            // update package

				// pulls
				{http.MethodGet, k8sRepo + "/pulls"}, // check existing k8s PRs (found one, don't open anew)
				{http.MethodGet, ocRepo + "/pulls"},  // check existing openshift PRs
				{http.MethodPost, ocRepo + "/pulls"}, // open new openshift PR
			}, map[mockRequest]mockRequester{
				// k8s
				{http.MethodGet, "/repos/k8s-operatorhub/community-operators/git/ref/heads/main"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, fmt.Sprintf(`{
						"object": {
							"sha": %q
						}
					}`, k8sMainSHA)
				},
				{http.MethodPatch, "/repos/some-fork/community-operators/git/refs/heads/main"}: func(r *http.Request, body string) (int, string) {
					assert.Contains(t, body, fmt.Sprintf(`"sha":%q`, k8sMainSHA))
					assert.Contains(t, body, `"force":true`)
					return http.StatusOK, `{}`
				},
				{http.MethodGet, "/repos/some-fork/community-operators/git/ref/heads/release-v1.2.3"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, `{
						"object": {
							"sha": "unexpected"
						}
					}`
				},
				{http.MethodPatch, "/repos/some-fork/community-operators/git/refs/heads/release-v1.2.3"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, `{}`
				},
				{http.MethodGet, "/repos/some-fork/community-operators/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, fmt.Sprintf(`{"sha": %q}`, k8sPackageSHA)
				},
				{http.MethodGet, "/repos/some-fork/community-operators/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, fmt.Sprintf(`{"sha": %q}`, k8sCSVSHA)
				},
				{http.MethodPut, "/repos/some-fork/community-operators/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, `{}`
				},
				{http.MethodPut, "/repos/some-fork/community-operators/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, `{}`
				},

				// openshift
				{http.MethodGet, "/repos/redhat-openshift-ecosystem/community-operators-prod/git/ref/heads/main"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, fmt.Sprintf(`{
						"object": {
							"sha": %q
						}
					}`, openshiftMainSHA)
				},
				{http.MethodPatch, "/repos/some-fork/community-operators-prod/git/refs/heads/main"}: func(r *http.Request, body string) (int, string) {
					assert.Contains(t, body, fmt.Sprintf(`"sha":%q`, openshiftMainSHA))
					assert.Contains(t, body, `"force":true`)
					return http.StatusOK, `{}`
				},
				{http.MethodGet, "/repos/some-fork/community-operators-prod/git/ref/heads/release-v1.2.3"}: func(r *http.Request, body string) (int, string) {
					return http.StatusNotFound, ``
				},
				{http.MethodPost, "/repos/some-fork/community-operators-prod/git/refs"}: func(r *http.Request, body string) (int, string) {
					assert.Contains(t, body, fmt.Sprintf(`"sha":%q`, openshiftMainSHA))
					assert.Contains(t, body, `"ref":"refs/heads/release-v1.2.3"`)
					return http.StatusOK, `{}`
				},
				{http.MethodGet, "/repos/some-fork/community-operators-prod/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, fmt.Sprintf(`{"sha": %q}`, openshiftPackageSHA)
				},
				{http.MethodGet, "/repos/some-fork/community-operators-prod/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, fmt.Sprintf(`{"sha": %q}`, openshiftCSVSHA)
				},
				{http.MethodPut, "/repos/some-fork/community-operators-prod/contents/operators/ibmcloud-operator/ibmcloud-operator.package.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, `{}`
				},
				{http.MethodPut, "/repos/some-fork/community-operators-prod/contents/operators/ibmcloud-operator/1.2.3/ibmcloud_operator.v1.2.3.clusterserviceversion.yaml"}: func(r *http.Request, body string) (int, string) {
					return http.StatusOK, `{}`
				},

				// pulls
				{http.MethodGet, "/repos/k8s-operatorhub/community-operators/pulls"}: func(r *http.Request, body string) (int, string) {
					// k8s PR already exists
					query := url.Values{"head": []string{"some-fork:release-v1.2.3"}}
					assert.Contains(t, r.URL.RawQuery, query.Encode())
					return http.StatusOK, fmt.Sprintf(`[
						{"html_url": %q}
					]`, existingK8sPR)
				},
				{http.MethodGet, "/repos/redhat-openshift-ecosystem/community-operators-prod/pulls"}: func(r *http.Request, body string) (int, string) {
					// openshift PR does not exist
					return http.StatusOK, `[]`
				},
				{http.MethodPost, "/repos/redhat-openshift-ecosystem/community-operators-prod/pulls"}: func(r *http.Request, body string) (int, string) {
					assert.Contains(t, body, `"head":"some-fork:release-v1.2.3"`)
					assert.Contains(t, body, `"base":"main"`)
					return http.StatusOK, fmt.Sprintf(`{"html_url":%q}`, newOpenShiftPR)
				},
			})
			var output bytes.Buffer
			deps := Deps{
				Output: &output,
				GitHub: &GitHub{doRequest: func(req *http.Request) (*http.Response, error) {
					recorder := httptest.NewRecorder()
					handler.ServeHTTP(recorder, req)
					return recorder.Result(), nil
				}},
			}
			err = run(args, deps)
			assert.Equal(t, strings.TrimPrefix(tc.expectOut, "\n"), output.String())
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

type mockRequest struct {
	Method string
	Path   string
}

type mockRequester func(r *http.Request, body string) (int, string)

type mockServerHandler struct {
	testingT        *testing.T
	currentRequest  int
	requests        []mockRequest
	responses       map[mockRequest]mockRequester
	unusedResponses map[mockRequest]bool
}

// newMockServerHandler returns a mock HTTP server handler for the given requests and responses
//
// expectRequestSeq is a list of expected requests in the order they are expected.
// responses is a map of a request's HTTP method & URL path to a response generator.
func newMockServerHandler(t *testing.T, expectRequests []mockRequest, responses map[mockRequest]mockRequester) http.Handler {
	t.Helper()
	m := &mockServerHandler{
		testingT:        t,
		requests:        expectRequests,
		responses:       responses,
		unusedResponses: make(map[mockRequest]bool),
	}
	for k := range responses {
		m.unusedResponses[k] = true
	}

	t.Cleanup(func() {
		t.Helper()
		assert.Equal(t, len(expectRequests), m.currentRequest, "Number of handled requests must equal expected request count")

		if !assert.Empty(t, m.unusedResponses, "Found responses that were not used. Fix or remove these:") {
			for req := range m.unusedResponses {
				t.Logf("Unused: %#v", req)
			}
		}
	})
	return m
}

func (m *mockServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.testingT.Helper()
	mockReq := mockRequest{
		Method: r.Method,
		Path:   r.URL.Path,
	}
	if assert.Less(m.testingT, m.currentRequest, len(m.requests), "Received more requests than expected. Extra request:", mockReq) {
		assert.Equal(m.testingT, m.requests[m.currentRequest], mockReq)
	}
	delete(m.unusedResponses, mockReq)
	m.currentRequest++

	requester, ok := m.responses[mockReq]
	if !assert.True(m.testingT, ok, "Request type not found: %#v", mockReq) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if r.Body == nil {
		r.Body = ioutil.NopCloser(bytes.NewReader(nil))
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	assert.NoError(m.testingT, err)

	status, responseBody := requester(r, string(requestBody))
	w.WriteHeader(status)
	fmt.Fprint(w, responseBody)
}
