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

package trigger

import (
	"github.com/apache/incubator-openwhisk-client-go/whisk"

	v1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
)

func deleteTrigger(wskclient *whisk.Client, obj *v1.Trigger, triggerName string, params whisk.KeyValueArr) {
	log := clog.WithValues("namespace", obj.Namespace, "name", obj.Name)

	otrigger, _, _ := wskclient.Triggers.Get(triggerName)
	if otrigger != nil {
		log.Info("deleting old trigger")

		if obj.Spec.Feed != "" {
			// Delete the old feed
			otrigger, _, _ := wskclient.Triggers.Get(triggerName)

			if otrigger != nil && otrigger.Annotations != nil {
				feedName, err := ow.GetValueString(otrigger.Annotations, "feed")
				if err == nil {
					feedQName, err := ow.ParseQualifiedName(feedName, "_")
					if err == nil {
						log.Info("deleting previous feed")

						// trigger feed already exists so first lets delete it and then recreate it
						deleteFeedAction(wskclient, triggerName, feedQName, params)
					}
				}
			}
		}

		wskclient.Triggers.Delete(triggerName) // TODO: should not ignore error
	}
}

func deleteFeedAction(client *whisk.Client, triggerName string, feedQName ow.QualifiedName, params whisk.KeyValueArr) error {
	namespace := client.Namespace

	params = append(params, whisk.KeyValue{Key: "authKey", Value: client.AuthToken})
	params = append(params, whisk.KeyValue{Key: "lifecycleEvent", Value: "DELETE"})
	params = append(params, whisk.KeyValue{Key: "triggerName", Value: "/" + namespace + "/" + triggerName})

	parameters := make(map[string]interface{})
	for _, keyVal := range params {
		parameters[keyVal.Key] = keyVal.Value
	}

	client.Namespace = feedQName.Namespace
	_, _, err := client.Actions.Invoke(feedQName.EntityName, parameters, true, true)
	client.Namespace = namespace

	if err != nil {
		clog.Error(err, "failed to invoke the feed when deleting trigger feed with error message (ignored)")
	}

	return err
}
