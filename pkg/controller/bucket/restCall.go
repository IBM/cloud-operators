package bucket

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"time"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
)

func (r *ReconcileBucket) updateBucket(bucket *ibmcloudv1alpha1.Bucket, token string, serverInstanceID string, checkExistOnly bool) (bool, error, string) {
	retnMessage := ""
	statusChange := false
	urlPrefix, _ := getEndpointURL(bucket.Spec.Resiliency, bucket.Spec.Location, bucket.Spec.BucketType)
	epString := fmt.Sprintf("https://%s/%s", urlPrefix, bucket.GetObjectMeta().GetAnnotations()["BucketName"])

	log.Info("Call rest call ", "restAPI", epString)
	restClient := http.Client{
		Timeout: time.Second * 300,
	}
	// anno := bucket.ObjectMeta.GetAnnotations()
	instanceid := serverInstanceID // anno["ExternalInstanceID"]
	u, _ := url.ParseRequestURI(epString)
	urlStr := u.String()
	var out bytes.Buffer
	if bucket.Spec.StorageClass != "" {
		out = getStorageClassSpec(bucket)
	}
	req, _ := http.NewRequest("PUT", urlStr, &out)
	if checkExistOnly {
		req, _ = http.NewRequest("GET", urlStr, &out)
	}

	// log.Info("", "out", out)
	authString := fmt.Sprintf("%s", token)
	req.Header.Set("Authorization", authString)
	// log.Info("Checking", "authString", authString)
	req.Header.Set("ibm-service-instance-id", instanceid)
	log.Info("Checking", "instanceid", instanceid)

	raw, err := restClient.Do(req)
	if err != nil {
		log.Info("Rest Call return ", "error", err)
		return statusChange, err, retnMessage
	}
	log.Info("Create Bucket return ", "StatusCode", raw.StatusCode)
	if raw.StatusCode != 200 {
		log.Info("Create Bucket return ", "error", raw.Body)
		if raw.StatusCode == 409 {
			// removeBucket(context.Background(), bucket, urlPrefix, token)
			return true, fmt.Errorf("This bucket name already exists in IBM Cloud Object Storage. Retrying ...."), retnMessage
		} else if raw.StatusCode == 404 && checkExistOnly {
			return true, fmt.Errorf("This bucket: %s does not exists", bucket.GetObjectMeta().GetAnnotations()["BucketName"]), retnMessage
		}
		return statusChange, err, retnMessage
	}
	if checkExistOnly {
		log.Info("Bucket exists", "name", bucket.GetObjectMeta().GetAnnotations()["BucketName"])
		updateAnnotation := false
		corsChanged := false
		retentionChanged := false
		if bucket.Spec.BindOnly {
			corsChanged, retentionChanged = r.checkBindOnlyDefault(bucket, serverInstanceID, token)
			log.Info("checkBindingDefault", "cors", corsChanged, "retention", retentionChanged)

		}
		if CheckCORS(bucket) || corsChanged {
			log.Info("CorsRule had changed")
			accessCorsRule(bucket, instanceid, urlPrefix, token, "PUT", ibmcloudv1alpha1.CORSRule{})
			updateAnnotation = true
		}

		if CheckRetentionPolicy(bucket) || retentionChanged {
			log.Info("Retention Policy had changed")
			updateAnnotation = true
			retErr, _ := accessRetentionPolicy(bucket, instanceid, urlPrefix, token, "PUT")
			if retErr != nil {
				log.Info("CreateRetention", "error", retErr)

				retnMessage = retErr.Error()
			}
			statusChange = true
		}
		if updateAnnotation {
			r.updateAnnotations(bucket, serverInstanceID)
		}
		return statusChange, nil, retnMessage
	}
	log.Info("Done creating", "bucketName is ", bucket.GetObjectMeta().GetAnnotations()["BucketName"])
	accessCorsRule(bucket, instanceid, urlPrefix, token, "PUT", ibmcloudv1alpha1.CORSRule{})
	accessRetentionPolicy(bucket, instanceid, urlPrefix, token, "PUT")

	return statusChange, nil, retnMessage
}

func accessCorsRule(bucket *ibmcloudv1alpha1.Bucket, instanceid string, urlPrefix string, token string, method string, _restoreCorsRule ibmcloudv1alpha1.CORSRule) (error, ibmcloudv1alpha1.CORSRule) {
	// Method: PUT https://<bucket endpoint>/<bucketname>?cors=ibmcloudv1alpha1
	inputRules := &_restoreCorsRule
	if reflect.DeepEqual(_restoreCorsRule, ibmcloudv1alpha1.CORSRule{}) {
		inputRules = &bucket.Spec.CORSRules
	}

	_corsRule := &ConfigCORSRule{AllowedHeader: inputRules.AllowedHeader, AllowedOrigin: inputRules.AllowedOrigin, AllowedMethod: inputRules.AllowedMethods}
	/* corsconfiguration.ConfigCORSRule.AllowedOrigin = inputRuls.AllowedOrigin
	corsconfiguration.ConfigCORSRule.AllowedHeader = inputRuls.AllowedHeader
	corsconfiguration.ConfigCORSRule.AllowedMethods = inputRuls.AllowedMethods */
	corsconfiguration := &CORSConfiguration{CORSRule: *_corsRule}
	log.Info("", "configuration", corsconfiguration)
	xmlBlob, _ := xml.Marshal(corsconfiguration)

	epString := fmt.Sprintf("https://%s/%s?cors=", urlPrefix, bucket.GetObjectMeta().GetAnnotations()["BucketName"])
	log.Info("", "epString", epString)
	restClient := http.Client{
		Timeout: time.Second * 300,
	}
	u, _ := url.ParseRequestURI(epString)
	urlStr := u.String()
	req, _ := http.NewRequest(method, urlStr, bytes.NewBuffer(xmlBlob))

	if method == "PUT" {
		os.Stdout.Write(xmlBlob)
		h := md5.New()
		xmlBlobStr := fmt.Sprintf("%s", xmlBlob)
		io.WriteString(h, xmlBlobStr)
		md5Str := base64.StdEncoding.EncodeToString(h.Sum(nil))
		log.Info("", "md5", md5Str)
		req.Header.Set("content-MD5", md5Str)
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("ibm-service-instance-id", instanceid)
	res, err2 := restClient.Do(req)
	if method == "PUT" {
		if err2 != nil {
			log.Info("Rest Call return ", "error", err2)
			return err2, ibmcloudv1alpha1.CORSRule{}
		}
		return nil, ibmcloudv1alpha1.CORSRule{}
	}
	if err2 == nil {
		currentCORS := ibmcloudv1alpha1.CORSRule{}
		if res.StatusCode == 404 {
			return nil, currentCORS
		}
		corsConfiguration := CORSConfiguration{}
		body, _ := ioutil.ReadAll(res.Body)

		xml.Unmarshal([]byte(body), &corsConfiguration)
		currentCORS.AllowedHeader = corsConfiguration.CORSRule.AllowedHeader
		currentCORS.AllowedMethods = corsConfiguration.CORSRule.AllowedMethod
		currentCORS.AllowedOrigin = corsConfiguration.CORSRule.AllowedOrigin
		return nil, currentCORS
	}
	return err2, ibmcloudv1alpha1.CORSRule{}
}

func accessRetentionPolicy(bucket *ibmcloudv1alpha1.Bucket, instanceid string, urlPrefix string, token string, method string) (error, ibmcloudv1alpha1.RetentionPolicy) {

	// Method: PUT https://<bucket endpoint>/<bucketname>?cors=
	retentionPolicy := &bucket.Spec.RetentionPolicy
	if retentionPolicy.DefaultRetentionDay > retentionPolicy.MaximumRetentionDay ||
		retentionPolicy.DefaultRetentionDay < retentionPolicy.MinimumRetentionDay {
		return fmt.Errorf("Default Retention value much be between Maximum and Minimum Retention"), ibmcloudv1alpha1.RetentionPolicy{}
	}

	_maxRetentionDay := &RetentionDay{Days: retentionPolicy.MaximumRetentionDay}
	_minRetentionDay := &RetentionDay{Days: retentionPolicy.MinimumRetentionDay}
	_defRetentionDay := &RetentionDay{Days: retentionPolicy.DefaultRetentionDay}
	protectionconfiguration := &ProtectionConfiguration{Status: "Retention", MinimumRetention: *_minRetentionDay, MaximumRetention: *_maxRetentionDay, DefaultRetention: *_defRetentionDay}
	log.Info("", "configuration", protectionconfiguration)
	xmlBlob, _ := xml.Marshal(protectionconfiguration)

	epString := fmt.Sprintf("https://%s/%s?protection=", urlPrefix, bucket.GetObjectMeta().GetAnnotations()["BucketName"])
	log.Info("", "epString", epString)
	restClient := http.Client{
		Timeout: time.Second * 300,
	}
	u, _ := url.ParseRequestURI(epString)
	urlStr := u.String()
	req, _ := http.NewRequest(method, urlStr, bytes.NewBuffer(xmlBlob))

	if method == "PUT" {
		os.Stdout.Write(xmlBlob)
		h := md5.New()
		xmlBlobStr := fmt.Sprintf("%s", xmlBlob)
		io.WriteString(h, xmlBlobStr)
		md5Str := base64.StdEncoding.EncodeToString(h.Sum(nil))
		log.Info("", "md5", md5Str)
		req.Header.Set("content-MD5", md5Str)
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("ibm-service-instance-id", instanceid)

	rst, err2 := restClient.Do(req)
	if method == "PUT" {
		if err2 != nil {
			log.Info("Rest Call return ", "error", err2)
			return err2, ibmcloudv1alpha1.RetentionPolicy{}
		}
		return nil, ibmcloudv1alpha1.RetentionPolicy{}
	}
	if err2 == nil {
		currentretPolicy := ibmcloudv1alpha1.RetentionPolicy{}
		if rst.StatusCode == 404 {
			return nil, currentretPolicy
		}
		protectionConfiguration := ProtectionConfiguration{}
		body, _ := ioutil.ReadAll(rst.Body)

		xml.Unmarshal([]byte(body), &protectionConfiguration)
		currentretPolicy.MinimumRetentionDay = protectionConfiguration.MinimumRetention.Days
		currentretPolicy.MaximumRetentionDay = protectionConfiguration.MaximumRetention.Days
		currentretPolicy.DefaultRetentionDay = protectionConfiguration.DefaultRetention.Days
		return nil, currentretPolicy
	}
	return nil, ibmcloudv1alpha1.RetentionPolicy{}
}

func removeBucket(context context.Context, bucket *ibmcloudv1alpha1.Bucket, urlPrefix string, token string) error {

	epString := fmt.Sprintf("https://%s/%s", urlPrefix, bucket.GetObjectMeta().GetAnnotations()["BucketName"])
	log.Info("Remove Bucket", "epString", epString)
	restClient := http.Client{
		Timeout: time.Second * 120,
	}
	u, _ := url.ParseRequestURI(epString)
	urlStr := u.String()
	req, _ := http.NewRequest("DELETE", urlStr, nil)
	req.Header.Set("Authorization", token)
	req.Header.Set("Host", urlPrefix)
	raw, err := restClient.Do(req)

	if err != nil {
		log.Info("Delete Bucket failed %s", "error", err)
		return err
	}
	log.Info("Remove Bucket", "StatueCode", raw.StatusCode)
	if raw.StatusCode > 300 && raw.StatusCode != 404 && raw.StatusCode != 403 {

		return fmt.Errorf("Remove Bucket failed, Statue Code = %d", raw.StatusCode)
	}
	log.Info("restcall return null")
	return nil
}

func removeObjectsInBucket(ctx context.Context, bucket *ibmcloudv1alpha1.Bucket, urlPrefix string, token string, deleteObjects Delete) error {
	epString := fmt.Sprintf("https://%s/%s?delete=", urlPrefix, bucket.GetObjectMeta().GetAnnotations()["BucketName"])
	restClient := http.Client{
		Timeout: time.Second * 300,
	}
	u, _ := url.ParseRequestURI(epString)
	urlStr := u.String()
	xmlBlob, _ := xml.Marshal(&deleteObjects)
	req, _ := http.NewRequest("POST", urlStr, bytes.NewBuffer(xmlBlob))
	req.Header.Set("Authorization", token)
	h := md5.New()
	xmlBlobStr := fmt.Sprintf("%s", xmlBlob)
	io.WriteString(h, xmlBlobStr)
	md5Str := base64.StdEncoding.EncodeToString(h.Sum(nil))

	req.Header.Set("Content-MD5", md5Str)
	_, err := restClient.Do(req)
	if err != nil {
		log.Info("restCall return ", "error", err)
		return err
	}
	return nil
}
func locateObjectsInBucket(ctx context.Context, bucket *ibmcloudv1alpha1.Bucket, urlPrefix string, token string) Delete {
	delete := Delete{}
	epString := fmt.Sprintf("https://%s/%s", urlPrefix, bucket.GetObjectMeta().GetAnnotations()["BucketName"])
	restClient := http.Client{
		Timeout: time.Second * 300,
	}
	u, _ := url.ParseRequestURI(epString)
	urlStr := u.String()

	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", token)
	res, err2 := restClient.Do(req)
	if err2 != nil {
		log.Info(bucket.GetObjectMeta().GetAnnotations()["BucketName"], "error ", err2)
		return delete
	}
	body, _ := ioutil.ReadAll(res.Body)
	bucketInfo := ListBucketResult{}
	xml.Unmarshal([]byte(body), &bucketInfo)
	for _, obj := range bucketInfo.Contents {
		objKey := obj.Key
		delete.Object = append(delete.Object, Object{Key: objKey})
	}
	// xmlBlob, _ := xml.Marshal(&delete)
	log.Info(bucket.GetObjectMeta().GetAnnotations()["BucketName"], "Returning to be Deleted objs", delete)
	return delete
}
