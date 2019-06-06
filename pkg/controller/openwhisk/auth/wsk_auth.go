/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package auth

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/apache/incubator-openwhisk-client-go/whisk"
)

// BmxNamespacesResponse is this...
type BmxNamespacesResponse struct {
	Subject    string                 `json:"subject"`
	Namespaces []BmxNamespaceResponse `json:"namespaces"`
}

// BmxNamespacesRequest is this...
type BmxNamespacesRequest struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// BmxNamespaceResponse is this...
type BmxNamespaceResponse struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
	Key  string `json:"key"`
}

// AuthenticateUserWithWsk gets the OpenWhisk apikey associated to the given UUA token
func AuthenticateUserWithWsk(apiHost string, uaaTokenOnly string, uaaRefreshToken string, insecure bool) (*BmxNamespacesResponse, *http.Response, error) {
	var reqURLStr = fmt.Sprintf("https://%s/%s", apiHost, "bluemix/v2/authenticate")

	reqNamespaces := &BmxNamespacesRequest{
		AccessToken:  uaaTokenOnly,
		RefreshToken: uaaRefreshToken,
	}

	var buf io.ReadWriter = new(bytes.Buffer)
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(reqNamespaces)

	if err != nil {
		whisk.Debug(whisk.DbgError, "json.Encode(%#v) error: %s\n", reqNamespaces, err)
		errStr := fmt.Sprintf("Error encoding request body: %s\n", err)
		werr := whisk.MakeWskError(errors.New(errStr), whisk.EXIT_CODE_ERR_GENERAL, whisk.DISPLAY_MSG, whisk.NO_DISPLAY_USAGE)
		return nil, nil, werr
	}

	httpReq, err := http.NewRequest("POST", reqURLStr, buf)
	if err != nil {
		whisk.Debug(whisk.DbgError, "http.NewRequest(POST, %s, buf) error: %s\n", reqURLStr, err)
		//errStr := wski18n.T("Error initializing request: {{.err}}", map[string]interface{}{"err": err})
		errStr := fmt.Sprintf("Error initializing request: %s", err)
		werr := whisk.MakeWskError(errors.New(errStr), whisk.EXIT_CODE_ERR_GENERAL, whisk.DISPLAY_MSG, whisk.NO_DISPLAY_USAGE)
		return nil, nil, werr
	}
	httpReq.Header.Add("Content-Type", "application/json")
	httpReq.Header.Add("User-Agent", "kube-operator-openwhisk")
	if whisk.IsVerbose() {
		fmt.Println("REQUEST:")
		fmt.Printf("[%s]\t%s\n", httpReq.Method, httpReq.URL)
		if len(httpReq.Header) > 0 {
			fmt.Println("Req Headers")
			whisk.PrintJSON(httpReq.Header)
		}
		if httpReq.Body != nil {
			fmt.Println("Req Body")
			fmt.Println(httpReq.Body)
		}
	}

	var httpClient = makeHTTPClient(insecure)

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		whisk.Debug(whisk.DbgError, "HTTP Do() [req %s] error: %s\n", httpReq.URL.String(), err)
		werr := whisk.MakeWskError(err, whisk.EXIT_CODE_ERR_NETWORK, whisk.DISPLAY_MSG, whisk.NO_DISPLAY_USAGE)
		return nil, httpResp, werr
	}

	whisk.Verbose("RESPONSE:")
	whisk.Verbose("Got response with code %d\n", httpResp.StatusCode)
	if whisk.IsVerbose() && len(httpResp.Header) > 0 {
		fmt.Println("Resp Headers")
		whisk.PrintJSON(httpResp.Header)
	}

	// Read the response body
	data, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		whisk.Debug(whisk.DbgError, "ioutil.ReadAll(resp.Body) error: %s\n", err)
		werr := whisk.MakeWskError(err, whisk.EXIT_CODE_ERR_NETWORK, whisk.DISPLAY_MSG, whisk.NO_DISPLAY_USAGE)
		return nil, httpResp, werr
	}
	whisk.Verbose("Response body size is %d bytes\n", len(data))
	whisk.Verbose("Response body received:\n%s\n", string(data))

	var respBmxNamespaces = &BmxNamespacesResponse{}
	whisk.Debug(whisk.DbgInfo, "Parsing HTTP response into BmxNamespacesResponse\n")

	dc := json.NewDecoder(strings.NewReader(string(data)))
	dc.UseNumber()
	err = dc.Decode(respBmxNamespaces)

	// If the decode was successful, return the response without error; otherwise, the decode did not work, so the
	// server response was unexpected
	if !whisk.IsHttpRespSuccess(httpResp) || err != nil || len(respBmxNamespaces.Subject) == 0 {
		whisk.Debug(whisk.DbgWarn, "Unsuccessful parse of HTTP response into BmxNamespacesResponse; parse error '%s'\n", string(data))
		errStr := fmt.Sprintf("Response status %d; Response %s", httpResp.StatusCode, string(data))
		werr := whisk.MakeWskError(errors.New(errStr), whisk.EXIT_CODE_ERR_NETWORK, whisk.DISPLAY_MSG, whisk.NO_DISPLAY_USAGE)
		return nil, httpResp, werr
	}

	whisk.Debug(whisk.DbgInfo, "Successful parse of HTTP response into BmxNamespacesResponse\n")
	whisk.Debug(whisk.DbgInfo, "Bluemix namespaces: %#q\n", respBmxNamespaces)
	return respBmxNamespaces, httpResp, nil
}

// FindAuthKey gets the key corresponding to the CF org and space
func FindAuthKey(bmxNamespaces *BmxNamespacesResponse, org string, space string) (string, error) {
	namespace := org + "_" + space
	var retAuthKey string

	for _, ns := range bmxNamespaces.Namespaces {
		if ns.Name == namespace {
			retAuthKey = ns.UUID + ":" + ns.Key
		}
	}
	if len(retAuthKey) == 0 {
		whisk.Debug(whisk.DbgError, "No auth key found for namespace `%s`\n", namespace)
		//errStr := wski18n.T("Namespace '{{.name}}' is not in the list of entitled namespaces", map[string]interface{}{"name": bmxflags.namespace})
		errStr := fmt.Sprintf("Target Org '%s' and Space '%s' do not have an auth key; if Space '%s' was recently created, try again in a couple minutes.", org, space, space)
		whiskErr := whisk.MakeWskError(errors.New(errStr), whisk.EXIT_CODE_ERR_GENERAL, whisk.DISPLAY_MSG, whisk.NO_DISPLAY_USAGE)
		return "", whiskErr
	}

	return retAuthKey, nil
}

func makeHTTPClient(useInsecureConnection bool) *http.Client {
	// Disable certificate checking in the dev environment if in insecure mode
	if useInsecureConnection {
		whisk.Debug(whisk.DbgInfo, "Disabling certificate checking.\n")

		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}

		http.DefaultClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}
	return http.DefaultClient
}
