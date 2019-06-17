/*
 * Copyright 2017-2018 IBM Corporation
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

package test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/apache/incubator-openwhisk-client-go/whisk"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	slog            = logf.Log.WithName("test")
	org             = ""
	space           = ""
	region          = ""
	resourceGroup   = ""
	apikey          = os.Getenv("BLUEMIX_API_KEY")
	auth            = os.Getenv("OW_AUTH")
	apihost         = os.Getenv("OW_APIHOST")
	uaaAccessToken  = ""
	uaaRefreshToken = ""
	ts              = time.Now().Unix()
)

func init() {
	if apikey == "" {
		panic("set BLUEMIX_API_KEY to run tests")
	}

	usr, err := user.Current()
	if err == nil {
		raw, err := ioutil.ReadFile(usr.HomeDir + "/.bluemix/config.json")
		if err == nil {
			var config map[string]interface{}
			json.Unmarshal(raw, &config)

			region = config["Region"].(string)
			if rgfields, ok := config["ResourceGroup"]; ok {
				resourceGroup = rgfields.(map[string]interface{})["Name"].(string)
			}

		}
		raw2, err := ioutil.ReadFile(usr.HomeDir + "/.bluemix/.cf/config.json")
		if err == nil {
			var config map[string]interface{}
			json.Unmarshal(raw2, &config)

			if orgfields, ok := config["OrganizationFields"]; ok {
				org = orgfields.(map[string]interface{})["Name"].(string)
			}

			if spacefields, ok := config["SpaceFields"]; ok {
				space = spacefields.(map[string]interface{})["Name"].(string)
			}

			if accessToken, ok := config["AccessToken"]; ok {
				uaaAccessToken = accessToken.(string)
			}

			if refreshToken, ok := config["RefreshToken"]; ok {
				uaaRefreshToken = refreshToken.(string)
			}
		}
	}

	if org == "" || space == "" || region == "" || uaaAccessToken == "" || uaaRefreshToken == "" {
		panic("set current bx target to run tests")
	}

	if apihost == "" || auth == "" {
		// Trying ~/.wskprops
		usr, err := user.Current()
		if err == nil {
			raw, err := ioutil.ReadFile(usr.HomeDir + "/.wskprops")
			if err == nil {
				lines := strings.Split(string(raw), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					kv := strings.Split(line, "=")
					if len(kv) != 2 {
						// Invalid format; skip
						continue
					}
					if kv[0] == "AUTH" {
						auth = kv[1]
					} else if kv[0] == "APIHOST" {
						apihost = kv[1]
					}
				}
			}
		}
	}
}

// InvokeAction invokes the given action
func InvokeAction(wskclient *whisk.Client, actionName string, payload interface{}) (string, error) {
	result, _, err := wskclient.Actions.Invoke(actionName, payload, true, true)
	if err != nil {
		rootErr := err.(*whisk.WskError).RootErr
		return "", rootErr
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
