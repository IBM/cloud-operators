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

package bucket

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("bucket")

const bucketFinalizer = "bucket.ibmcloud.ibm.com"

type ConfigCORSRule struct {
	AllowedOrigin string   `xml:"AllowedOrigin"`
	AllowedHeader string   `xml:"AllowedHeader"`
	AllowedMethod []string `xml:"AllowedMethod"`
}

// CORSConfiguration : CORS rules
type CORSConfiguration struct {
	CORSRule ConfigCORSRule `xml:"CORSRule"`
}

// RetentionConfiguration : Retention Policy
type ProtectionConfiguration struct {
	Status           string       `xml:"Status"`
	MinimumRetention RetentionDay `xml:"MinimumRetention"`
	MaximumRetention RetentionDay `xml:"MaximumRetention"`
	DefaultRetention RetentionDay `xml:"DefaultRetention"`
}
type RetentionDay struct {
	Days int `xml:"Days"`
}

// CreateBucketConfiguration : The postbody for the Bucket creation
type CreateBucketConfiguration struct {
	LocationConstraint string `xml:"LocationConstraint"`
}

const (
	retryInterval time.Duration = time.Second * 30 //time.Second * 15
	pingInterval  time.Duration = time.Minute * 15 //time.Second * 15
)

// ContainsFinalizer check for Finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.Bucket) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, bucketFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete streams finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.Bucket) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == bucketFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new Bucket Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBucket{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// WaitItem wait for Binding Object to be ready
type WaitItem struct {
	name      string
	namespace string
	waitfor   string
}

// WaitQLock lock for WaitItem Queue
type WaitQLock struct {
	sync.RWMutex
	waitBuckets []WaitItem // types.NamespacedName
}

var wlocker = new(WaitQLock)

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("bucket-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 30})
	if err != nil {
		return err
	}

	// Watch for changes to Bucket
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Bucket{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for Binding Object to be created, inqueue only when the Binding Object has not been created,
	// If bunding object is not active state, the the normal queueu will be used with RetryInterval
	mapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {

			wlocker.Lock()
			defer wlocker.Unlock()
			if len(wlocker.waitBuckets) == 0 {
				return []reconcile.Request{}
			}

			var retReq []reconcile.Request

			for _, n := range wlocker.waitBuckets {
				if n.waitfor == a.Meta.GetName() {
					q := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      n.name,
							Namespace: n.namespace,
						},
					}
					retReq = append(retReq, q)
				}

			}
			return retReq

		})

	c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: mapFn,
	})

	return nil
}

var _ reconcile.Reconciler = &ReconcileBucket{}

// ReconcileBucket reconciles a Bucket object
type ReconcileBucket struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Bucket object and makes changes based on the state read
// and what is in the Bucket.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=buckets/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=buckets/finalizers,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileBucket) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Bucket instance
	instance := &ibmcloudv1alpha1.Bucket{}
	err := r.Get(context.Background(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	logRecorder(instance, "Bucket Reconcile", instance.ObjectMeta.Name)

	token, err := r.getIamToken(instance.ObjectMeta.Namespace)

	if err != nil || token == "" {
		return r.updateStatus(instance, resv1.ResourceStateFailed, err, true)
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			logRecorder(instance, "calling Finalizer")
			if !instance.Spec.BindOnly {
				retry, _err := r.callFinalizer(instance, token)
				log.Info("CallFinalizer return", "retry", retry, "error", _err)
				// remove our finalizer from the list and update it.
				if _err == nil || retry == false {
					instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
					if err := r.Update(context.Background(), instance); err != nil {
						logRecorder(instance, "Error removing finalizers", err.Error())
					}
					return reconcile.Result{}, nil
				}
				if retry {
					r.updateStatus(instance, resv1.ResourceStateDeleting, _err, true)
					return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 30}, nil
				}

			} else {
				// Put the bucket back to its original state
				r.restoreBindOnlyCorsDefaults(instance, token)
				instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
				if err := r.Update(context.Background(), instance); err != nil {
					logRecorder(instance, "Error removing finalizers", err.Error())
				}
				return reconcile.Result{}, nil
			}

		}
		return reconcile.Result{}, nil
	}

	// First, make sure there is the binding object specified by the BindingFrom
	// Reading bindings
	bindingObj, err := r.getBindingObject(instance)
	if err != nil {
		if strings.Contains(instance.Status.Message, "not found") && instance.Status.State != resv1.ResourceStateWaiting {
			wlocker.QueueInstance(instance)
			return r.updateStatus(instance, resv1.ResourceStateWaiting, err, false) // Dont retry, the watcher will watch for the creation of BindingObject, set for PingInterval
		}
		return r.updateStatus(instance, resv1.ResourceStateWaiting, err, true) // Binding Object is not ready yet, retry in retryInterval
	}
	wlocker.DeQueueInstance(instance)

	serviceObj, err := r.getServiceInstance(bindingObj)

	// If serviceInstance is not there, or is not ready, then wait for it, this can only happen when service instance is health checked and is being recreated
	if err != nil {
		return r.updateStatus(instance, resv1.ResourceStateWaiting, err, true) // retry in retryInterval, theoretically, it can be put in the watch queue, in case it take time to redo the creation
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.BucketStatus{}) {
		instance.Status.State = resv1.ResourceStatePending
		instance.Status.Message = "Processing Resource"
		if err := r.Status().Update(context.Background(), instance); err != nil {
			log.Info("Update", instance.Status.State, err)
			return reconcile.Result{}, err
		}
		// Set the Default Bucket creation parameters, and attach a 8 chars rand string
		r.setDefault(instance, serviceObj.Status.InstanceID, token)

	}

	// Set Finalizer
	if !ContainsFinalizer(instance) {
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, bucketFinalizer)
		if err := r.Update(context.Background(), instance); err != nil {
			logRecorder(instance, "Error setting Finalizer")
			return reconcile.Result{}, err
		}
	}

	// Set OwnerReference
	if err := controllerutil.SetControllerReference(bindingObj, instance, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// State Pending cover the initial creation or retrying
	// Pending: initial creation
	// Retrying: Create Bucket has failed
	// Waiting: was waiting for the services/binding object
	if instance.Status.State == resv1.ResourceStatePending || instance.Status.State == resv1.ResourceStateRetrying ||
		instance.Status.State == resv1.ResourceStateWaiting {
		// Do the Bucket Creation
		return r.runReconcile(instance, token, serviceObj.Status.InstanceID, instance.Spec.BindOnly)
	}

	// Health Check
	if instance.Status.State == resv1.ResourceStateOnline {
		// Locate the Cloud Object Storage instance ID
		recreate := false
		anno := instance.GetObjectMeta().GetAnnotations()
		logRecorder(instance, "Status", instance.Status.State, "Msg", instance.Status.Message, "Anno", anno["ExternalInstanceID"])

		// If Service has been re-created
		if anno["ExternalInstanceID"] != serviceObj.Status.InstanceID && instance.Status.State == resv1.ResourceStateOnline {
			// try to remove the Bucket, might not be successful
			r.callFinalizer(instance, token)
			instance.Status.State = resv1.ResourceStatePending
			instance.Status.Message = "Recreating buckets"
			if err := r.Status().Update(context.Background(), instance); err != nil {
				logRecorder(instance, "update to state", instance.Status.State, err)
				return reconcile.Result{}, nil
			}
			r.setDefault(instance, serviceObj.Status.InstanceID, token)
			recreate = true
		} else {
			logRecorder(instance, "bucket still there", instance.GetObjectMeta().GetAnnotations()["BucketName"])
			statusChange, err, retnMsg := r.updateBucket(instance, token, serviceObj.Status.InstanceID, true)
			log.Info("UpdateBucket", "err", err, "retnMsg", retnMsg)
			// the bucket is gone
			if err != nil {
				// if it is BindOnly, update the status and retrun to retry,
				if instance.Spec.BindOnly {
					return r.updateStatus(instance, resv1.ResourceStateRetrying, err, true)
				}
				// Change the Bucket name ( with different randString as the originally one might be still not available)
				r.updateBucketName(instance)
				recreate = true
			} else {
				// Bucket is still there, remember to update status
				if retnMsg != "" {
					log.Info("Update Messages......")
					instance.Status.Message = "Bucket:" + instance.GetObjectMeta().GetAnnotations()["BucketName"] + " update failed. Reason:" + retnMsg
					if err := r.Status().Update(context.Background(), instance); err != nil {
						logRecorder(instance, "update to state", instance.Status.State, err)
						return reconcile.Result{}, err
					}
				} else if statusChange {
					if strings.Contains(instance.Status.Message, "failed") {
						instance.Status.Message = "Bucket:" + instance.GetObjectMeta().GetAnnotations()["BucketName"]
						if err := r.Status().Update(context.Background(), instance); err != nil {
							logRecorder(instance, "update to state", instance.Status.State, err)
							return reconcile.Result{}, err
						}

					}
				}
			}
		}

		// Recreate handles bucket is missing or Service is being recreated
		if recreate {
			instance.Status.State = resv1.ResourceStatePending
			instance.Status.Message = "Processing Resource"
			if err := r.Status().Update(context.Background(), instance); err != nil {
				logRecorder(instance, "update to state", instance.Status.State, err)
				return reconcile.Result{}, err
			}
			return r.runReconcile(instance, token, serviceObj.Status.InstanceID, instance.Spec.BindOnly)

		}
	}

	return reconcile.Result{Requeue: true, RequeueAfter: pingInterval}, nil
}

func (r *ReconcileBucket) runReconcile(bucket *ibmcloudv1alpha1.Bucket, token string, serverInstanceID string, checkExistOnly bool) (reconcile.Result, error) {
	retry, err, retnMsg := r.updateBucket(bucket, token, serverInstanceID, bucket.Spec.BindOnly)

	logRecorder(bucket, "Calling updateBucket", "bucketname", bucket.GetObjectMeta().GetAnnotations()["BucketName"], "retry", retry, "error", err)

	if err != nil {
		if retry {
			r.updateStatus(bucket, resv1.ResourceStatePending, err, retry)
			return reconcile.Result{Requeue: true, RequeueAfter: retryInterval}, nil
		} else {
			r.updateStatus(bucket, resv1.ResourceStateFailed, err, retry)
			return reconcile.Result{}, nil
		}
	} else if retnMsg != "" {
		r.updateAnnotations(bucket, serverInstanceID)
		r.updateStatus(bucket, resv1.ResourceStateOnline, fmt.Errorf("Bucket:%s ,%s", bucket.GetObjectMeta().GetAnnotations()["BucketName"], retnMsg), retry)
		return reconcile.Result{Requeue: true, RequeueAfter: pingInterval}, nil
	} else {
		r.updateAnnotations(bucket, serverInstanceID)
		r.updateStatus(bucket, resv1.ResourceStateOnline, fmt.Errorf("Bucket: %s online", bucket.GetObjectMeta().GetAnnotations()["BucketName"]), retry)
		return reconcile.Result{Requeue: true, RequeueAfter: pingInterval}, nil

	}
}

func (r *ReconcileBucket) getBindingObject(instance *ibmcloudv1alpha1.Bucket) (*ibmcloudv1alpha1.Binding, error) {
	binding := &ibmcloudv1alpha1.Binding{}
	bucketNameSpace := instance.ObjectMeta.Namespace
	gerr := r.Get(context.Background(), client.ObjectKey{
		Namespace: bucketNameSpace,
		Name:      instance.Spec.BindingFrom.Name,
	}, binding)

	if gerr != nil {
		log.Info("", "error", gerr)
		return nil, gerr
	} else if binding.Status.State != resv1.ResourceStateOnline {
		return &ibmcloudv1alpha1.Binding{}, fmt.Errorf("Binding Object %s not ready", instance.Spec.BindingFrom.Name)
	}
	return binding, nil
}

func (r *ReconcileBucket) updateStatus(instance *ibmcloudv1alpha1.Bucket, state string, err error, retry bool) (reconcile.Result, error) {
	message := err.Error()

	instance.Status.State = state
	instance.Status.Message = message
	if err := r.Status().Update(context.Background(), instance); err != nil {
		log.Info("Error updating status", state, err.Error())
	}
	if retry {
		log.Info("updateStatus", "error", err, "retry", retry, "state", state)
		if state == resv1.ResourceStateFailed {
			log.Info("updateStatue", "instance", instance.ObjectMeta.Name, "requeue After", "60 seconds")
			return reconcile.Result{Requeue: true, RequeueAfter: retryInterval}, nil

		}
		return reconcile.Result{}, err

	}
	return reconcile.Result{Requeue: true, RequeueAfter: pingInterval}, nil

}

func (r *ReconcileBucket) getServiceInstance(instance *ibmcloudv1alpha1.Binding) (*ibmcloudv1alpha1.Service, error) {
	serviceNameSpace := instance.ObjectMeta.Namespace

	serviceInstance := &ibmcloudv1alpha1.Service{}
	err := r.Get(context.Background(), types.NamespacedName{Name: instance.Spec.ServiceName, Namespace: serviceNameSpace}, serviceInstance)
	if err != nil {
		return &ibmcloudv1alpha1.Service{}, err
	}
	return serviceInstance, nil
}

func (r *ReconcileBucket) callFinalizer(bucket *ibmcloudv1alpha1.Bucket, token string) (bool, error) {

	urlPrefix, _ := getEndpointURL(bucket.Spec.Resiliency, bucket.Spec.Location, bucket.Spec.BucketType)

	deleteObjs := locateObjectsInBucket(context.Background(), bucket, urlPrefix, token)

	if &deleteObjs != nil && deleteObjs.Object != nil {
		// There are objects in the to-be-removed bucket, if the KeepIfNotEmpty flag is set, then remove should fail
		// Otherwise, just remove all the object and continue
		if !bucket.Spec.KeepIfNotEmpty {
			removeObjectsInBucket(context.Background(), bucket, urlPrefix, token, deleteObjs)
		} else {

			rmErr := fmt.Errorf("Bucket %s contains object", bucket.GetObjectMeta().GetAnnotations()["BucketName"])
			log.Info("Bucket contains object", "err", rmErr)
			return true, rmErr
		}
	}

	if err := removeBucket(context.Background(), bucket, urlPrefix, token); err != nil {
		log.Info("Failed to remove", "error", err)
		return true, err
	}
	return false, nil
}

// getIamToken : this function require the seed-secret-tokens secret to be available.
func (r *ReconcileBucket) getIamToken(secretnamespace string) (string, error) {
	secretName := "seed-secret-tokens"

	secret := &v1.Secret{}

	if err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: secretnamespace}, secret); err != nil {
		if secretnamespace != "default" {
			if err := r.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: secretName}, secret); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	return string(secret.Data["access_token"]), nil
}
func (r *ReconcileBucket) checkBindOnlyDefault(bucket *ibmcloudv1alpha1.Bucket, serviceInstanceID string, token string) (bool, bool) {
	// For BindOnly, still check if need to update CorsRULE or Retentions
	updateCORSRule := false
	urlPrefix, _ := getEndpointURL(bucket.Spec.Resiliency, bucket.Spec.Location, bucket.Spec.BucketType)
	err1, corsRule := accessCorsRule(bucket, serviceInstanceID, urlPrefix, token, "GET", ibmcloudv1alpha1.CORSRule{})
	log.Info("checkBindOnlyDefault", "corsRule", corsRule, "yaml", bucket.Spec.CORSRules)
	if err1 == nil {
		if !reflect.DeepEqual(bucket.Spec.CORSRules, corsRule) {
			updateCORSRule = true
		}
	}
	updateRetention := false
	err2, retentionPolicy := accessRetentionPolicy(bucket, serviceInstanceID, urlPrefix, token, "GET")
	log.Info("checkBindOnlyDefault", "retentionPolicy", retentionPolicy, "yaml", bucket.Spec.RetentionPolicy)
	if err2 == nil {
		if !reflect.DeepEqual(retentionPolicy, bucket.Spec.RetentionPolicy) {
			updateRetention = true
		}
	}

	return updateCORSRule, updateRetention
}

func (r *ReconcileBucket) setDefault(bucket *ibmcloudv1alpha1.Bucket, serviceInstanceID string, token string) error {

	if bucket.Spec.Location == "" {
		bucket.Spec.Location = "us-south"

	}
	if bucket.Spec.Resiliency == "" {
		bucket.Spec.Resiliency = "regional"

	}

	anno := bucket.GetObjectMeta().GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
		anno["ExternalInstanceID"] = ""
	} else {
		anno["ExternalInstanceID"] = ""
	}

	if !bucket.Spec.BindOnly {
		if reflect.DeepEqual(bucket.Spec.CORSRules, ibmcloudv1alpha1.CORSRule{}) {
			bucket.Spec.CORSRules = ibmcloudv1alpha1.CORSRule{AllowedHeader: "*", AllowedOrigin: "*", AllowedMethods: []string{"POST", "GET", "PUT"}}

		}
	} else {
		urlPrefix, _ := getEndpointURL(bucket.Spec.Resiliency, bucket.Spec.Location, bucket.Spec.BucketType)
		err1, corsRule := accessCorsRule(bucket, serviceInstanceID, urlPrefix, token, "GET", ibmcloudv1alpha1.CORSRule{})
		if err1 == nil {
			corsData, _ := json.Marshal(corsRule)
			anno["BindOnlyCORSDefault"] = string(corsData)
		} else {
			log.Info("BundonlyCORSDefault", "error", err1)
		}

	}

	if anno == nil {
		anno = make(map[string]string)
		anno["ExternalInstanceID"] = ""
	} else {
		anno["ExternalInstanceID"] = ""
	}

	if bucket.Spec.BindOnly {
		anno["BucketName"] = bucket.ObjectMeta.Name
	} else {
		anno["BucketName"] = bucket.ObjectMeta.Name + "-" + RandString(8)
	}

	bucket.GetObjectMeta().SetAnnotations(anno)

	if err := r.Update(context.Background(), bucket); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileBucket) updateAnnotations(bucket *ibmcloudv1alpha1.Bucket, serviceInstanceID string) {
	anno := bucket.GetObjectMeta().GetAnnotations()
	anno["ExternalInstanceID"] = serviceInstanceID

	corsData, _ := json.Marshal(bucket.Spec.CORSRules)
	anno["OrigCORSRule"] = string(corsData)

	retData, _ := json.Marshal(bucket.Spec.RetentionPolicy)
	anno["OrigRetentionPolicy"] = string(retData)

	bucket.GetObjectMeta().SetAnnotations(anno)
	if err := r.Update(context.Background(), bucket); err != nil {
		log.Info("Update Annotation", "error", err)
	}
}

func (r *ReconcileBucket) storeBindOnlyCorsDefaults(bucket *ibmcloudv1alpha1.Bucket, corsDefault ibmcloudv1alpha1.CORSRule) {
	anno := bucket.GetObjectMeta().GetAnnotations()
	corsData, _ := json.Marshal(bucket.Spec.CORSRules)
	anno["BindOnlyCORSDefault"] = string(corsData)

	bucket.GetObjectMeta().SetAnnotations(anno)
	if err := r.Update(context.Background(), bucket); err != nil {
		log.Info("Update Annotation", "error", err)
	}
}

func (r *ReconcileBucket) restoreBindOnlyCorsDefaults(bucket *ibmcloudv1alpha1.Bucket, token string) {
	anno := bucket.GetObjectMeta().GetAnnotations()
	corString := anno["BindOnlyCORSDefault"]

	defaultCors := ibmcloudv1alpha1.CORSRule{}
	json.Unmarshal([]byte(corString), &defaultCors)
	log.Info("Calling accessCoreRule", "bindOnlyDefautls", defaultCors)
	urlPrefix, _ := getEndpointURL(bucket.Spec.Resiliency, bucket.Spec.Location, bucket.Spec.BucketType)
	if reflect.DeepEqual(defaultCors, ibmcloudv1alpha1.CORSRule{}) {
		accessCorsRule(bucket, anno["ExternalInstanceID"], urlPrefix, token, "DELETE", defaultCors)

	} else {
		accessCorsRule(bucket, anno["ExternalInstanceID"], urlPrefix, token, "PUT", defaultCors)

	}

}

func (r *ReconcileBucket) updateBucketName(bucket *ibmcloudv1alpha1.Bucket) {
	anno := bucket.GetObjectMeta().GetAnnotations()
	if bucket.Spec.BindOnly {
		anno["BucketName"] = bucket.ObjectMeta.Name
	} else {
		anno["BucketName"] = bucket.ObjectMeta.Name + "-" + RandString(8)

	}
	bucket.GetObjectMeta().SetAnnotations(anno)
	if err := r.Update(context.Background(), bucket); err != nil {
		log.Info("Update Annotation", "error", err)
	}
}
