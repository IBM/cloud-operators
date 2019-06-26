package bucket

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
)

// ListBucketResult is used to realize Objects in a Bucket
type ListBucketResult struct {
	Name      string `xml:"Name"`
	Prefix    string `xml:"Prefix"`
	Marker    string `xml:"Marker"`
	MaxKeys   int    `xml:"MaxKeys"`
	Delimiter string `xml:"Delimiter"`
	Contents  []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
		Size         int    `xml:"Size"`
		Owner        struct {
			ID          string `xml:"ID"`
			DisplayName string `xml:"DisplayName"`
		} `xml:"Owner"`
		StorageClass string `xml:"StorageClass"`
	}
	IsTruncated bool `xml:"IsTruncated"`
}

// Delete is used to realize the tobe Deleted Object in a Bucket
type Delete struct {
	Object []Object `xml:"Object"`
}

// Object is used to keep key info
type Object struct {
	Key string `xml:"Key"`
}

func getEndpointURL(resiliency string, location string, buckettype string) (string, error) {
	urlPrefix := getURLPrefix(strings.ToLower(buckettype))
	locationstr := fmt.Sprintf("%s", location)
	if resiliency == "cross-region" || resiliency == "Cross Region" {
		url, err := getCrossRegionEndpoint(strings.ToLower(locationstr), urlPrefix)
		return url, err
	}
	if resiliency == "regional" {
		url, err := getRegionalEndpoint(strings.ToLower(locationstr), urlPrefix)
		return url, err
	}
	if resiliency == "single-site" {
		url, err := getSingleSiteEndpoint(strings.ToLower(locationstr), urlPrefix)
		return url, err
	}
	return "", nil

}

func getURLPrefix(buckettype string) string {
	var prefix string
	switch buckettype {
	case "private":
		prefix = "s3.private."
	default:
		prefix = "s3."
	}
	return prefix
}

func getRegionalEndpoint(locationtop string, urlPrefix string) (string, error) {
	// Need to get a dynamic way of getting endpoints
	return urlPrefix + locationtop + ".cloud-object-storage.appdomain.cloud", nil
}

func getCrossRegionEndpoint(locationtop string, urlPrefix string) (string, error) {
	// Need to get a dynamic way of getting endpoints

	switch locationtop {
	case "us", "us-geo":
		return urlPrefix + "us.cloud-object-storage.appdomain.cloud", nil

	case "eu", "eu-geo":
		return urlPrefix + "eu.cloud-object-storage.appdomain.cloud", nil

	case "ap", "ap-geo":

		return urlPrefix + "ap.cloud-object-storage.appdomain.cloud", nil

	default:
		return "", fmt.Errorf("Unrecognized region: %s", locationtop)
	}

}
func getSingleSiteEndpoint(locationtop string, urlPrefix string) (string, error) {
	// Need to get a dynamic way of getting endpoints
	return urlPrefix + locationtop + ".cloud-object-storage.appdomain.cloud", nil

}

// GetCloudEndpoint : Return endpoint URL based on region
func GetCloudEndpoint(region string) (string, error) {
	// TODO: use bx regions
	switch region {
	case "eu-de":
		return "api.eu-de.bluemix.net", nil
	case "au-syd":
		return "api.au-syd.bluemix.net", nil
	case "us-east":
		return "api.us-east.bluemix.net", nil
	case "us-south":
		return "api.ng.bluemix.net", nil
	case "eu-gb":
		return "api.eu-gb.bluemix.net", nil
	default:
		return "", fmt.Errorf("Unrecognized region: %s", region)
	}
}

func getStorageClassSpec(bucket *ibmcloudv1alpha1.Bucket) bytes.Buffer {
	loc := bucket.Spec.Location
	if strings.Contains(bucket.Spec.Location, "-geo") {
		_locs := strings.Split(bucket.Spec.Location, "-")
		loc = _locs[0]
	}
	var bucketConf CreateBucketConfiguration
	switch bucket.Spec.StorageClass {
	case "Standard":

		bucketConf.LocationConstraint = loc + "-standard"
	case "Vault":
		bucketConf.LocationConstraint = loc + "-vault"
	case "Cold Vault":
		bucketConf.LocationConstraint = loc + "-cold"
	case "Flex":
		bucketConf.LocationConstraint = loc + "-flex"
	}
	xmlBlob, _ := xml.Marshal(&bucketConf)
	log.Info("xmlBlob", "", bucketConf)
	out := *bytes.NewBuffer(xmlBlob)
	return out
}

func (l *WaitQLock) QueueInstance(instance *ibmcloudv1alpha1.Bucket) {
	l.Lock()
	defer l.Unlock()
	var repeat = false
	log.Info("QueueInstance", "namespace", instance.GetNamespace(), "name", instance.GetName(), "waitfor", instance.Spec.BindingFrom.Name)

	for _, n := range l.waitBuckets {
		log.Info("==>", "namespace", instance.GetNamespace(), "name", instance.GetName(), "waitfor", instance.Spec.BindingFrom.Name)

		if n.namespace == instance.GetNamespace() && n.name == instance.GetName() && n.waitfor == instance.Spec.BindingFrom.Name {

			repeat = true
			log.Info("QueueInstance", "repeat", repeat)
		}
	}
	if !repeat {
		l.waitBuckets = append(l.waitBuckets, WaitItem{namespace: instance.GetNamespace(), name: instance.GetName(), waitfor: instance.Spec.BindingFrom.Name})
	}
}

func (l *WaitQLock) DeQueueInstance(instance *ibmcloudv1alpha1.Bucket) {
	l.Lock()
	defer l.Unlock()
	filteredQueue := l.waitBuckets[:0]
	for _, n := range l.waitBuckets {
		if n.namespace != instance.GetNamespace() || n.name != instance.GetName() || n.waitfor != instance.Spec.BindingFrom.Name {
			filteredQueue = append(filteredQueue, n)
		}
	}
	log.Info("DequeueInstance", "lenof", len(filteredQueue))
	l.waitBuckets = filteredQueue
}
func getGUID(instanceID string) string {
	instanceIDs := strings.Split(instanceID, ":")
	if len(instanceIDs) > 1 {
		return instanceIDs[len(instanceIDs)-3]

	}
	return instanceID

}

const charset = "abcdefghijklmnopqrstuvwxyz"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func RandStringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func RandString(length int) string {
	return RandStringWithCharset(length, charset)
}

func logRecorder(bucket *ibmcloudv1alpha1.Bucket, keysAndValues ...interface{}) {
	log.Info(bucket.ObjectMeta.Name, "info", keysAndValues)
}

func CheckCORS(bucket *ibmcloudv1alpha1.Bucket) bool {
	annos := bucket.GetObjectMeta().GetAnnotations()
	oldCorsInfo := ibmcloudv1alpha1.CORSRule{}
	err := json.Unmarshal([]byte(annos["OrigCORSRule"]), &oldCorsInfo)
	log.Info("CheckCORS", "prev", oldCorsInfo, "new", bucket.Spec.CORSRules)
	if err == nil {
		if oldCorsInfo.AllowedHeader != bucket.Spec.CORSRules.AllowedHeader {
			log.Info("Header changed", "prev", oldCorsInfo.AllowedHeader, "now", bucket.Spec.CORSRules.AllowedHeader)
			return true
		}
		if oldCorsInfo.AllowedOrigin != bucket.Spec.CORSRules.AllowedOrigin {
			log.Info("Origin changed", "prev", oldCorsInfo.AllowedOrigin, "now", bucket.Spec.CORSRules.AllowedOrigin)
			return true
		}

		if !reflect.DeepEqual(oldCorsInfo.AllowedMethods, bucket.Spec.CORSRules.AllowedMethods) {
			log.Info("Method changed", "prev", oldCorsInfo.AllowedMethods, "now", bucket.Spec.CORSRules.AllowedMethods)
			return true
		}
	}

	return false
}

func CheckRetentionPolicy(bucket *ibmcloudv1alpha1.Bucket) bool {
	annos := bucket.GetObjectMeta().GetAnnotations()
	oldRetentionPolicy := ibmcloudv1alpha1.RetentionPolicy{}
	err := json.Unmarshal([]byte(annos["OrigRetentionPolicy"]), &oldRetentionPolicy)
	if err == nil {

		if !reflect.DeepEqual(oldRetentionPolicy, bucket.Spec.RetentionPolicy) {
			log.Info("Retention Policy changed", "prev", oldRetentionPolicy, "now", bucket.Spec.RetentionPolicy)
			return true
		}
	}

	return false
}
