/*
 * Copyright 2019 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package ibmcloud

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	context "github.com/ibm/cloud-operators/pkg/context"
	"github.com/ibm/cloud-operators/pkg/lib/secret"
)

// ContentReader is an agnostic util reader to fetch content from web or local path or potentially other places.
type ContentReader struct {
	URLReader
	LocalReader
}

// URLReader is used by ContentReader
type URLReader struct {
}

// LocalReader struct is used to read content from a local file
type LocalReader struct {
}

// ReadURL reads content from an URL
func (urlReader *URLReader) ReadURL(url string, token string) (content []byte, retCode int, err error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 500, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, 500, err
	}

	// check status code and return if auth issues or similar
	if resp.StatusCode >= 400 {
		return b, resp.StatusCode, fmt.Errorf("ReadURL() status: %d resp: %s", resp.StatusCode, string(b))
	}

	return b, resp.StatusCode, nil
}

// ReadLocal reads content from a local file
func (localReader *LocalReader) ReadLocal(path string) ([]byte, error) {
	cont, err := ioutil.ReadFile(path)
	return cont, err
}

// Read reads content from a http URL or a local file
func Read(ctx context.Context, url string) ([]byte, error) {
	if strings.HasPrefix(url, "http") {
		// we assume that the most common case is for a public url without token
		data, rc, err := new(ContentReader).URLReader.ReadURL(url, "")

		if err != nil {
			// if rc indicates bad request, unauthorized or forbidden we try to use the iam token
			if rc == 400 || rc == 401 || rc == 403 {
				token, err := secret.GetSecretValue(ctx, "seed-secret", "access_token", true)
				if err != nil {
					return nil, err
				}

				data, _, err = new(ContentReader).URLReader.ReadURL(url, string(token))
			} else if rc == 404 && strings.HasPrefix(url, "https://raw.github.") {
				// Retry with personal token stored in secret
				parts := strings.Split(strings.Replace(url, "https://", "", -1), "/")
				if len(parts) < 4 {
					return nil, fmt.Errorf("Invalid URL: %s", url)
				}
				secretname := parts[0] + "-" + parts[1] + "-" + parts[2]
				value, e2 := secret.GetSecretValue(ctx, secretname, "token", true)
				if e2 != nil {
					// Try global token
					value, e2 = secret.GetSecretValue(ctx, parts[0], "token", true)
					if e2 != nil {
						return nil, e2
					}
				}
				data, _, err = new(ContentReader).URLReader.ReadURL(url, "token "+string(value))
			}
		}
		return data, err
	}
	return new(ContentReader).LocalReader.ReadLocal(url)
}
