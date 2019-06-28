# IBM Cloud Operator User Guide

This guide provides information on how to use the IBM Cloud Operator to provision and bind
IBM Cloud public instances.

## Managing Services

### Creating a Service

To create an instance of an IBM public cloud service, create the following custom resource:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
    name: myservice
spec:
    plan: <PLAN>
    serviceClass: <SERVICE_CLASS>
```    

to find the value for `<SERVICE_CLASS>`, you can list the names of all IBM public cloud 
services with the command:

```bash
ibmcloud catalog service-marketplace
```

once you find the `<SERVICE_CLASS>` name, you can list the available plans to select
a `<PLAN>` with the command:

```bash
ibmcloud catalog service <SERVICE_CLASS> | grep plan
```

After creating a service, you can find its status with:

```bash
kubectl get services.ibmcloud 
NAME           STATUS   AGE
myservice      Online   12s
```

#### Enabling Self-Healing

With self-healing enabled, when a service created by the operator is deleted out-of-band
(e.g. via `ibmcloud` CLI or IBM Cloud UI) then the service is automatically re-created by
the operator. This feature might be useful to ensure that a service instance is always available
even if it gets deleted accidentally or the current instance becomes unhealthy. However,
we do not reccomend using this feature with stateful services (e.g. databases).

To enable self-healing, add the following annotation to the service definition:

```yaml
annotations:
    ibmcloud.ibm.com/self-healing: enabled
```

for example, to create an instance of an IBM translator service with self-healing, use the following
example:

```yaml
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
  name: mytranslator
  namespace: default
  annotations:
    ibmcloud.ibm.com/self-healing: enabled
spec:
  plan: lite
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

### Self-healing for Bindings

Self-healing for bindings is always enabled. If credentials are removed out-of-band they are
automatically recreated.


### Deleting a Binding

To delete a binding with name `mybinding`, run:

```bash
kubectl delete binding.ibmcloud mybinding
```

Similarly to services, the operator uses finalizers to remove the custom resource
only after the service credentials are removed.
