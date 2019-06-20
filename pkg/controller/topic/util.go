package topic

import (
	"encoding/json"
	"fmt"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	rcontext "github.com/ibm/cloud-operators/pkg/context"
	common "github.com/ibm/cloud-operators/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
)

// Create is a type used for passing parameters to Rest calls for creation
type Create struct {
	Name              string                 `json:"name"`
	Partitions        int32                  `json:"partitions"`
	ReplicationFactor int32                  `json:"replicationFactor,omitempty"`
	Configs           map[string]interface{} `json:"configs,omitempty"`
}

func getTopic(kafkaAdminURL string, apiKey string, topic *ibmcloudv1alpha1.Topic) (common.RestResult, error) {
	epString := fmt.Sprintf("%s/admin/topics/%s", kafkaAdminURL, topic.Spec.TopicName)
	ret, resterr := common.RestCallFunc(epString, nil, "GET", "X-Auth-Token", apiKey, true)
	return ret, resterr
}

func deleteTopic(kafkaAdminURL string, apiKey string, topic *ibmcloudv1alpha1.Topic) (common.RestResult, error) {
	epString := fmt.Sprintf("%s/admin/topics/%s", kafkaAdminURL, topic.Spec.TopicName)
	ret, resterr := common.RestCallFunc(epString, nil, "DELETE", "X-Auth-Token", apiKey, true)
	return ret, resterr
}

func createTopic(ctx rcontext.Context, kafkaAdminURL string, apiKey string, topic *ibmcloudv1alpha1.Topic) (common.RestResult, error) {
	epString := fmt.Sprintf("%s/admin/topics", kafkaAdminURL)
	var topicObj Create
	topicObj.Name = topic.Spec.TopicName
	if topicObj.Partitions == 0 {
		topicObj.Partitions = 1
	} else {
		topicObj.Partitions = topic.Spec.NumPartitions
	}
	topicObj.ReplicationFactor = topic.Spec.ReplicationFactor

	if topic.Spec.Configs != nil {
		configMap := make(map[string]interface{})
		for _, kv := range topic.Spec.Configs {
			kvJSON, err := kv.ToJSON(ctx)
			if err != nil {
				return common.RestResult{}, err
			}
			configMap[kv.Name] = kvJSON
		}
		topicObj.Configs = configMap
	}
	topicBlob, _ := json.Marshal(&topicObj)

	// ("Topic Creation content ", "topicBlob", topicBlob)
	ret, resterr := common.RestCallFunc(epString, topicBlob, "POST", "X-Auth-Token", apiKey, true)
	return ret, resterr
}

func getKafkaAdminInfo(instance *ibmcloudv1alpha1.Topic, binding *corev1.Secret) (string, string, error) {
	kafkaAdminURL, ok := binding.Data["kafka_admin_url"]
	if !ok {
		return "", "", fmt.Errorf("missing kafka_admin_url")
	}

	apiKey, ok := binding.Data["api_key"]
	if !ok {
		return "", "", fmt.Errorf("missing kafka_admin_url")
	}

	return string(kafkaAdminURL), string(apiKey), nil
}
