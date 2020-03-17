
[![Build Status](https://travis-ci.com/IBM/cloud-operators.svg?branch=master)](https://travis-ci.com/IBM/cloud-operators)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/cloud-operators)](https://goreportcard.com/report/github.com/IBM/cloud-operators)
[![GoDoc](https://godoc.org/github.com/IBM/cloud-operators?status.svg)](https://godoc.org/github.com/IBM/cloud-operators)

# IBM Cloud Operator

The IBM Cloud Operator provides a simple Kubernetes CRD-Based API to provision and bind 
IBM public cloud services on your Kubernetes cluster. With this operator, you no longer need
out-of-band processes to consume IBM Cloud Services in your application; 
you can simply provide service and binding custom resources as part of your Kubernetes 
application templates and let the operator reconciliation logic ensure that the required 
resources are automatically created and maintained.

For a detailed guide on how to use the IBM Cloud Operator, see [IBM Cloud Operator User Guide](docs/user-guide.md).
 

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
ibmcloud target --cf -g default
```

This will use the IBM Cloud ResourceGroup `default`. To specify a different ResourceGroup, use the following command:
```bash
ibmcloud target -g <resource-group>
```

Notice that the `org` and `space` must be included, even if no Cloud Foundry services will be instantiated.

## Installing the operator

To install the latest release of the operator, run the following script:

```
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/install-operator.sh | bash 
```

The script above first creates an IBM Cloud API Key and stores it in a Kubernetes secret that can be
accessed by the operator, then it sets defaults such as the default resource group and region 
used to provision IBM Cloud Services; finally, it deploys the operator in your cluster. You can always override the defaults in the `Service` custom resource. If you prefer to create the secret and the defaults manually, consult the [IBM Cloud Operator documentation](docs/install.md).

### Using a ServiceId

To instantiate services and bindings on behalf of a ServiceId, set the environment variable `IC_APIKEY` to the `api-key` of the ServiceId. This can be obtained via the IBM Cloud Console of CLI.

Next log into the IBM Cloud account that owns the ServiceId and follow the instructions above.

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

### Service Yaml Elements

The `Service` yaml includes the following elements:

Field | Is required | Format/Type | Comments
----- | ------------|-------------|-----------------
 serviceClass | Yes | String | The type of service being instantiated
 plan | Yes | String |  The plan to use for service instantiation, set to `Alias` for linking to an existing service instance
 serviceClassType | Yes/No | String | Set to `CF` for Cloud Foundry services, omit otherwise
 externalName | No | [string]string | The name that appears for the instantiated service on the IBM Public Cloud Dashboard
 parameters | No | []Any | Parameters passed for service instantiation, the type could be anything (int, string, object, ...)
 tags | No | []String | Tags passed for service instantiation. These tags appear on the instance on the IBM Public Cloud Dashboard
 context | No | Context Type | Used to override default account context (see below)

Each `paramater` is treated as a `RawExtension` by the Operator and parsed into JSON.
The `plan` is set to `Alias` to link to an existing service instance (see [IBM Cloud Operator User Guide](docs/user-guide.md)
for more details).

_Notice that the `serviceClass`, `plan`, `serviceClassType`, and `externalName` fields are immutable. Immutability is not enforced
with an admission controller, so updates go through initially successfully. However, the controller intercepts such changes and
changes those fields back to their original values. So although it may seem that updates to those fields are accepted, they are
in fact reverted by the controller. In the future, we plan to implement updatability for `parameters`._

The IBM Cloud Operator needs an account context, which indicates the `api-key` and the details of the IBM Public Cloud
account to be used for service instantiation. The `api-key` is contained in a Secret called `seed-secrets` that is created
when the IBM Cloud Operator is installed. Details of the account (such as organization, space, resource group) are held in a
ConfigMap called `seed-defaults`. To find the secret and configmap the IBM Cloud Operator first looks at the namespace of the
resource being created, and if not found, in the default namespace. This account information can be overriden by using
the `context` field in the service yaml, with the following substructure:

Field | Is required | Format/Type 
----- | ------------|-------------
 org | No | String 
 space | No | String 
 region | No | String 
 resourceGroup | No | String 
 resourceGroupID | No | String
 resourceLocation | No | String 

To override any element, the user can simply indicate that element and omit the others.
If a resourceGroup is indicated, then the resourceGroupID must also be provided. This can be obtained with the
following command, and retrieving the field `ID`.

```bash
ibmcloud resource group <resourceGroup>
```


### Binding Yaml Elements

Field | Is required | Format/Type | Comments
----- | ------------|-------------|-----------------
 serviceName | Yes | String | The name of the Service resource corresponding to the service instance on which to create credentials
 serviceNamespace | No | String |  The namespace of the Service resource
 alias | No | String | The name of credentials, if this Binding is linking to existing credentials
 secretName | No | String | The name of the Secret to be created
 role | No | String | The role to be passed for credentials creation
 parameters | No | []Any | Parameters passed for credentials creation, the type could be anything (int, string, object, ...)
 
The `alias` field is used to link to an existing credentials (see [IBM Cloud Operator User Guide](docs/user-guide.md)
for more details). If the `secretName` is omitted, then the same name as the `Binding` itself is used. If the `role` is
omitted, then the Operator sets role to `Manager`, if that is supported by the service (and if not, it picks the first role
listed by the service). Most services support the `Manager` role.



## Learn more about how to contribute

- [contributions](./CONTRIBUTING.md)

## Troubleshooting

The [troubleshooting](docs/troubleshooting.md) section provides info on how
to debug your operator.

