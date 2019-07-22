
[![Build Status](https://travis-ci.com/IBM/cloud-operators.svg?branch=master)](https://travis-ci.com/IBM/cloud-operators)

# IBM Cloud Operator

The IBM Cloud Operator provides a simple Kubernetes CRD-Based API to provision and bind 
IBM public cloud services on your Kubernetes cluster. With this operator, you no longer need
out-of-band processes to consume IBM Cloud Services in your application; 
you can simply provide service and binding custom resources as part of your Kubernetes 
application templates and let the operator reconciliation logic ensure that the required 
resources are automatically created.

## Features

* **Service Provisioning** - supports provisioning for any service and plan available in the IBM Cloud catalog.

* **Bindings Management** - automatically creates secrets with the credentials to bind to
any provisioned service.

## Requirements

The operator can be installed on any Kubernetes cluster with version >= 1.11. 
You need an [IBM Cloud account](https://cloud.ibm.com/registration) and the 
[IBM Cloud CLI](https://cloud.ibm.com/docs/cli?topic=cloud-cli-getting-started).
You need also to have the [kubectl CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl/)  
already configured to access your cluster. Before installing the operator, you need to login to 
your IBM cloud account with the IBM Cloud CLI:

```bash
ibmcloud login
```

and set a default target environment for your resources with the command:

```bash
ibmcloud target --cf
```

## Installing the operator

To install the latest release of the operator, run the following script:

```
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/install-operator.sh | bash 
```

The script above first creates an IBM Cloud API Key and stores it in a Kubernetes secret that can be
accessed by the operator, then it sets defaults such as the default resource group and region 
used to provision IBM Cloud Services; finally, it deploys the operator in your cluster. You can always override the defaults in the `Service` custom resource. If you prefer to create the secret and the defaults manually, consult the [IBM Cloud Operator documentation](docs/install.md).

## Removing the operator

To remove the operator, run the following script:

```
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/uninstall-operator.sh | bash 
```

## Using the IBM Cloud Operator

You can create an instance of an IBM public cloud service using the following custom resource:

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

You can find [additional samples](config/samples), and more information on 
[using the operator](docs/user-guide.md) in the operator documentation.

## Learn more about how to contribute

- [contributions](./CONTRIBUTING.md)

## Troubleshooting

The [troubleshooting](docs/troubleshooting.md) section provides info on how
to debug your operator.

