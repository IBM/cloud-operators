# Bucket Controller 
apiVersion: ibmcloud.ibm.com/v1alpha1

## IMPORTANT


<em>Bucket controller is used to control life-cycle of Cloud Objec Storage Bucket </em>


### <a name="bucket"></a>- Bucket Controller Schema
Bucket name will be the name of the CR plus a ramdom character string (8 chars long)
* BucketName can be retrieve by checking the CR's metadata->annotations["BucketName"]

```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Bucket
metadata:
  annotations:
    BucketName: mytestbucket0615
```

* If the bucket was removed outside of the Lifecycle of the controller, the bucket will be created with name plus different random strings at the end
* Object inside Bucket will be removed when the deleting the controller, KeepIfNotEmpty flag can be used to avoid accidentally removing of non-empty bucket. With this flag, the Deleting action will fail and will stay in "Deleting" status, until user manual remove the object(s) inside the bucket. Or remove the KeepIfNotEmpty flag from the yaml spec and use `kubectl apply` to change the desired action.
* The location, resilency cannot be changed without removing and recreating the bucket.
* The CORS rules and RetentionPolicy can be changed by using "kubectl apply"
* `bindOnly` is used to bind to existing bucket. You can also use this to change the cors rule and retention policy of existing bucket. Removing the binconly CR will not remove the bucket, but the original CORS rule and Policy will be restored. `Note: Once you create a retention policy it can not be deleted.` To understand the `Retention Policy`, please reference [Immutable Object Storage](https://cloud.ibm.com/docs/services/cloud-object-storage/basics?topic=cloud-object-storage-immutable)  

#### Schema
```
apiVersion: ibmcloud.ibm.com/v1beta1
kind: Bucket
metadata:
  name: <name of the CR>
spec:
  bindingFrom:
    name: <name of the binding CR>
  resiliency: <Possible value: Cross Region, Regional, Single Site>
  location: <Possible value: depend on the resuleicy, please see attached table>
  storageClass: <Possible value: Standard, Value, Cold Value, Flex>
  bindOnly: <true, false(default): bind to existing bucket>
  corsRules:
    allowedOrigin: <string>
    allowedHeader: <string>
    allowedMethoded: 
        - POST
        - GET
        - PUT
  retentionPolicy:
    defaultRetentionDay: <integer, must be a number between Maximum Retention Day and Minimum Retention Day>
    maximumRetentionDay: <integer>
    minimumRetentionDay: <integer>

```
### Example
```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Bucket
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: cos4seedb-bucket-jadeyliu-014
spec:
  bindingFrom:
    name: cos4seedb
  bucketname: cos4seedb-bucket-jadeyliu-014
  resiliency: regional
  location: us-south
  storageclass: Cold Vault
  corsRules:
    allowedOrigin : "w1.ibm.com"
    allowedHeader : "*"
    allowedMethods :
      - POST
      - GET
      - PUT
  retentionPolicy :
    minimumRetentionDay: 20
    maximumRetentionDay: 40
    defaultRetentionDay: 25
```
