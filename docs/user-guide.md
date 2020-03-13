# IBM Cloud Operator User Guide

This guide provides information on how to use the IBM Cloud Operator to provision and bind
IBM Cloud public instances.

## Managing Services

### Creating a Service

To create an instance of an IBM public cloud service, create the following custom resource, `service.yaml`:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
    name: myservice
spec:
    plan: <PLAN>
    serviceClass: <SERVICE_CLASS>
```    

To find the value for `<SERVICE_CLASS>`, you can list the names of all IBM public cloud 
services with the command:

```bash
ibmcloud catalog service-marketplace
```

Once you find the `<SERVICE_CLASS>` name, you can list the available plans to select
a `<PLAN>` with the command:

```bash
ibmcloud catalog service <SERVICE_CLASS> | grep plan
```

To create the service:
```bash
kubectl apply -f service.yaml
```

After creating a service, you can find its status with:

```bash
kubectl get services.ibmcloud 
NAME           STATUS   AGE
myservice      Online   12s
```

When a service created by the operator is deleted out-of-band (e.g. via `ibmcloud` CLI or IBM Cloud UI) then the service is automatically re-created by the operator. This may take a few minutes because the IBM Cloud Operator runs at regular intervals, every few minutes.

#### Services requiring global region

Some combinations of service and plan require a `global` region. If this is not the case, the following
error message appears in the `status` of the service resource:
```bash
Failed: No deployment found for service plan <plan> at location <location>. Valid location(s) are: ["global"]
```

Here's an example of how to set the region to `global`:
```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
  name: mycos
spec:
  plan: standard
  serviceClass: cloud-object-storage
  context:
    region: global
```

#### Services that provision slowly

Most services provision very rapidly and appear `Online` shortly after creation. However, some services can
take a while, such as 10 to 30mns. While they are provisioning the service resource will reflect
this state in its `status`. For example, a `Cloudant` resource will display `inactive` until it is fully provisioned.
Some services might even have say `failed` until they become `Online`.

During this time, the corresponding binding resource may also have a `Failed` status, because some services refuse
to create credentials while they are provisioning and return an error. 

In these cases, everything becomes `Online` by simply waiting for a while. The service eventually becomes `Online`, and so does
the binding.


#### Referencing an existing service

If a cloud service is already provisioned in your account, you can still create a service resource
that is linked to that resource. This can be useful for example when the service has been created
by an administrator, or the service is stateful and you need to maintain data associated with
the service, or there are multiple clusters using the service, but only one is actively managing the service
(creating and deleting instances) while the other clusters are only linking to that service.

To create a `Service` resource for an existing service instance in your account, you should use the name of the service
instance and the reserved plan name `Alias`. For example, if the service `mytranslator` already exists, you can
use the following custom resource to link it:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
  name: mytranslator
  namespace: default
spec:
  plan: Alias
  serviceClass: language-translator
```


The following would also work: 


```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
  name: mytranslator-alias
  namespace: default
spec:
  plan: Alias
  serviceClass: language-translator
  externalName: mytranslator
```

For CF-type services, the name is unique within a context (org & space), therefore the name is sufficient
to identify an existing service instance. 

For IAM-type services, multiple service instances can have the same name.
The example above will work only if there is one single instance of the service with that name. If multiple
service instances with the same name exist, you must add an annotation to identify the particular instance
you want to link to (in addition to specifying the name of the instance to link to).

To find the instance ID to use, you can list the current instances with the same name with the command:

```bash
ibmcloud resource service-instance <service-instance-name>
```

for example:

```bash
ibmcloud resource service-instance mytranslator
Retrieving service instance mytranslator in resource group default under account Paolo Dettori's Account as dettori@us.ibm.com...
Multiple service instances found
OK
                          
Name:                  mytranslator   
ID:                    crn:v1:bluemix:public:language-translator:us-south:a/0b5a00334eaf9eb9339d2ab48f20d7f5:e641000a-9108-45fb-b2e6-ab7e52acc962::   
GUID:                  e641000a-9108-45fb-b2e6-ab7e52acc962   
Location:              us-south   
Service Name:          language-translator   
Service Plan Name:     standard   
Resource Group Name:   default   
State:                 active   
Type:                  service_instance   
Sub Type:                 
Created at:            2019-07-02T01:26:19Z   
Updated at:            2019-07-02T01:26:19Z   
                          
Name:                  mytranslator   
ID:                    crn:v1:bluemix:public:language-translator:us-south:a/0b5a00334eaf9eb9339d2ab48f20d7f5:aa7e9eba-e997-4e26-9aef-8d80f933625d::   
GUID:                  aa7e9eba-e997-4e26-9aef-8d80f933625d   
Location:              us-south   
Service Name:          language-translator   
Service Plan Name:     lite   
Resource Group Name:   default   
State:                 active   
Type:                  service_instance   
Sub Type:                 
Created at:            2019-07-02T02:13:15Z   
Updated at:            2019-07-02T02:13:15Z 
```

In the example above, there are two instances with the same name. To identify
which one to use, you may look at the plan, which might be different, or the creation date.
Let's assume you want to use the first instance; then, simply copy the ID value into the 
`ibmcloud.ibm.com/instanceId` annotation. The resource definition for this example is then:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
  name: mytranslator
  namespace: default
  annotations:
    ibmcloud.ibm.com/instanceId: "crn:v1:bluemix:public:language-translator:us-south:a/0b5a00334eaf9eb9339d2ab48f20d7f5:e641000a-9108-45fb-b2e6-ab7e52acc962::"
spec:
  plan: Alias
  serviceClass: language-translator
```

### Deleting a Service

To delete a service with name `myservice`, run:

```bash
kubectl delete service.ibmcloud myservice
```

The operator uses kubernetes finalizers to manage the deletion of the custom resource.
The operator first removes the service from IBM Cloud, then removes the finalizer, and
at this point the custom resource should no longer be available in your cluster.\

```bash
kubectl get services.ibmcloud myservice
Error from server (NotFound): services.ibmcloud.ibm.com "myservice" not found
```

If the resource being deleted [is only linked to the service instance](#referencing-an-existing-service)
then deleting the resource will not delete the service instance.

## Managing Bindings

### Creating a Binding

You can bind to a service with name `myservice` using the following custom resource:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Binding
metadata:
    name: mybinding
spec:
    serviceName: myservice
```    

To find the status of your binding, you can run the command:

```bash
kubectl get bindings.ibmcloud 
NAME                 STATUS   AGE
mybinding            Online   25s
```

A `Binding` generates a secret with the same name as the binding resource and 
contains service credentials that can be consumed by your application.

```bash
kubectl get secrets
NAME                       TYPE                                  DATA   AGE
mybinding                  Opaque                                6      102s
```

When a binding and service are created in the same namespace, there is an ownership relationship. So when when the service is deleted, so is the binding. This relationship does not exist if they are not in the same namespace (since Kubernetes disallows it).
In this case, the binding needs to be deleted manually and will not be deleted when the services is deleted.

#### Referencing existing credentials

When many bindings are needed on the same service, it is possible to link to the same set of credentials on the service instance,
instead of creating new ones.

```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Binding
metadata:
  name: binding-translator-alias
spec:
  serviceName: mytranslator
  alias: binding-translator
```

The field `alias` indicates the name of the credentials to link to. This name must be unique, i.e. there cannot be other credentials
on the same service instance with the same name. If the binding is deleted, this does not cause the deletion of the
corresponding credentials.

### Deleting a Binding

To delete a binding with name `mybinding`, run:

```bash
kubectl delete binding.ibmcloud mybinding
```

Similarly to services, the operator uses finalizers to remove the custom resource
only after the service credentials are removed. When a binding is deleted, the corresponding secret is also deleted.

In order to refresh credentials on a service, the user can simply delete and recreate a binding.