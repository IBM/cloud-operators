<!-- HIDE operator hub -->

[![Build Status](https://travis-ci.com/IBM/cloud-operators.svg?branch=master)](https://travis-ci.com/IBM/cloud-operators)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/cloud-operators)](https://goreportcard.com/report/github.com/IBM/cloud-operators)
[![GoDoc](https://godoc.org/github.com/IBM/cloud-operators?status.svg)](https://godoc.org/github.com/IBM/cloud-operators)

# IBM Cloud Operator

<!-- END HIDE operator hub -->

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

## Set up the operator

To set up the operator after logging in with `ibmcloud`, run the below installer.

By default, the script will create a new API key for use in the operator. To use a custom API key, set the `IBMCLOUD_API_KEY` environment variable to the key.

#### Using a ServiceId

To instantiate services and bindings on behalf of a ServiceId, set the environment variable `IBMCLOUD_API_KEY` to the `api-key` of the ServiceId. This can be obtained via the IBM Cloud Console or [CLI](https://cloud.ibm.com/docs/iam?topic=iam-serviceids). Be sure to give proper access permissions to the ServiceId.

Next log into the IBM Cloud account that owns the ServiceId and follow the instructions above.

<!-- HIDE operator hub -->
### Install

To install the latest release of the operator, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash 
```

The above script stores an API key in a Kubernetes secret that can be accessed by the operator.
Next, it sets default values used in provisioning IBM Cloud Services, like the resource group and region.
You can override any default value in the `Service` custom resource.
Finally, the script deploys the operator in your cluster.

If you prefer to create the secret and the defaults manually, consult the [IBM Cloud Operator documentation](docs/install.md).

To install a specific version of the operator, you can pass a semantic version:

```bash
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- -v 0.0.0
```

### Uninstall

To remove the operator, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- delete
```

<!-- END HIDE operator hub -->

### Configure the operator for OpenShift

To configure the latest release for OpenShift before install, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- store-creds
```

The above script stores an API key in a Kubernetes secret that can be accessed by the operator.
Next, it sets default values used in provisioning IBM Cloud Services, like the resource group and region.
You can override any default value in the `Service` custom resource.

If you prefer to create the secret and the defaults manually, consult the [IBM Cloud Operator documentation](docs/install.md).

To configure with a specific version of the operator, you can pass a semantic version:

```bash
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- -v 0.0.0 store-creds
```

## Using the IBM Cloud Operator

You can create an instance of an IBM public cloud service using the following custom resource:

```yaml
apiVersion: ibmcloud.ibm.com/v1beta1
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
apiVersion: ibmcloud.ibm.com/v1beta1
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

### Service Properties

A `Service` includes the following properties:

| Field            | Required | Type       | Comments                                                                                                   |
|:-----------------|:---------|:-----------|:-----------------------------------------------------------------------------------------------------------|
| serviceClass     | Yes      | `string`   | The type of service being instantiated                                                                     |
| plan             | Yes      | `string`   | The plan to use for service instantiation, set to `Alias` for linking to an existing service instance      |
| serviceClassType | CF only  | `string`   | Set to `CF` for Cloud Foundry services, omit otherwise                                                     |
| externalName     | No       | `string`   | The name that appears for the instantiated service on the IBM Public Cloud Dashboard                       |
| parameters       | No       | `[]Param`  | Parameters passed for service instantiation, the type can be anything (number, string, object, ...)        |
| tags             | No       | `[]string` | Tags passed for service instantiation. These tags appear on the instance on the IBM Public Cloud Dashboard |
| context          | No       | `Context`  | Used to override default account context (see below)                                                       |

Each `parameter`'s value is treated as a raw value by the Operator and passed as-is.
The `plan` can be set to `Alias` to link to an existing service instance (see [IBM Cloud Operator User Guide](docs/user-guide.md)
for more details).

_Notice that the `serviceClass`, `plan`, `serviceClassType`, and `externalName` fields are immutable. Immutability is not enforced
with an admission controller, so updates go through initially successfully. However, the controller intercepts such changes and
changes those fields back to their original values. So although it may seem that updates to those fields are accepted, they are
in fact reverted by the controller. On the other hand, `parameters` and `tags` are updatable._

The IBM Cloud Operator needs an account context, which indicates the `api-key` and the details of the IBM Public Cloud
account to be used for service instantiation. The `api-key` is contained in a Secret called `secret-ibm-cloud-operator` that is created
when the IBM Cloud Operator is installed. Details of the account (such as organization, space, resource group) are held in a
ConfigMap called `config-ibm-cloud-operator`. To find the secret and configmap the IBM Cloud Operator first looks at the namespace of the
resource being created, and if not found, in a management namespace (see below for more details on management namespaces). If there is no management namespace, then the operator looks for the secret and configmap in the `default` namespace. 


The account information can be overriden by using
the `context` field in the service yaml, with the following substructure:

| Field            | Required | Type     |
|:-----------------|:---------|:---------|
| org              | No       | `string` |
| space            | No       | `string` |
| region           | No       | `string` |
| resourceGroup    | No       | `string` |
| resourceGroupID  | No       | `string` |
| resourceLocation | No       | `string` |

To override any element, the user can simply indicate that element and omit the others.
If a `resourceGroup` is indicated, then the `resourceGroupID` must also be provided. This can be obtained with the
following command, and retrieving the field `ID`.

```bash
ibmcloud resource group <resourceGroup>
```

#### Using a Management Namespace

Different Kubernetes namespaces can contain different secrets `secret-ibm-cloud-operator` and configmap `config-ibm-cloud-operator`, corresponding to different IBM Public Cloud accounts. So each namespace can be set up for a different account. 

In some scenarios, however, there is a need for hiding the `api-keys` from users. In this case, a management namespace can be set up that contains all the secrets and configmaps corresponding to each namespace, with a naming convention. 

To configure a management namespace named `safe`, there must be a configmap named `ibm-cloud-operator` created in the same namespace as the IBM Cloud Operator itself. This configmap indicates the name of the management namespace, in a field `namespace`. To create such a config map, execute the following:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: ibm-cloud-operator
  namespace: <namespace where IBM Cloud Operator has been installed>
  labels:
    app.kubernetes.io/name: ibmcloud-operator
data:
  namespace: safe
EOF
```

This configmap indicates to the operator where to find the management namespace, in this case `safe`.
Next the `safe` namespace needs to contain secrets and configmaps corresponding to each namespace that will contain services and bindings. The naming convention is as follows:

```
<namespace>-secret-ibm-cloud-operator
<namespace>-config-ibm-cloud-operator
```

These can be created similary to what is done with `make install`.

If we create a service or binding resource in a namespace `XYZ`, the IBM Cloud Operator first looks in the `XYZ` namespace to find `secret-ibm-cloud-operator` and `config-ibm-cloud-operator`, for account context. If they are missing in `XYZ`, it looks for the `ibm-cloud-operator` configmap in the namespace where the operator is installed, to see if there is a management namespace. If there is, it looks in the management namespace for the secret and configmap with the naming convention:
`XYZ-secret-ibm-cloud-operator` and `XYZ-config-ibm-cloud-operator`. If there is no management namespace, the operator looks in the `default` namespace for the secret and configmap (`secret-ibm-cloud-operator` and `config-ibm-cloud-operator`).



### Binding Properties

A `Binding` includes the following properties:

| Field            | Required | Type     | Comments                                                                                              |
|:-----------------|:---------|:---------|:------------------------------------------------------------------------------------------------------|
| serviceName      | Yes      | `string` | The name of the `Service` resource corresponding to the service instance on which to create credentials |
| serviceNamespace | No       | `string` | The namespace of the `Service` resource                                                                 |
| alias            | No       | `string` | The name of credentials, if this `Binding` is linking to existing credentials                           |
| secretName       | No       | `string` | The name of the `Secret` to be created                                                                  |
| role             | No       | `string` | The role to be passed for credentials creation                                                        |
| parameters       | No       | `[]Any`  | Parameters passed for credentials creation, the type could be anything (int, string, object, ...)     |
 
The `alias` field is used to link to an existing credentials (see [IBM Cloud Operator User Guide](docs/user-guide.md)
for more details). If the `secretName` is omitted, then the same name as the `Binding` itself is used. If the `role` is
omitted, then the operator sets role to `Manager`, if that is supported by the service (and if not, it picks the first role
listed by the service). Most services support the `Manager` role.

## Learn more about how to contribute

- [contributions](./CONTRIBUTING.md)

## Troubleshooting

The [troubleshooting](docs/troubleshooting.md) section provides info on how
to debug your operator.
