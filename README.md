
[![Build Status](https://travis-ci.com/IBM/cloud-operators.svg?branch=master)](https://travis-ci.com/IBM/cloud-operators)

# IBM Cloud Operator
The IBM Cloud Operator provides a Kubernetes CRD-Based API to manage the lifecycle of IBM public cloud services. This operator allows to provision and bind any of the 130+ IBM public cloud services from your
Kubernetes cluster, using Custom Resource Definitions (CRDs) for Services and Bindings.

## Supported Features

* **Supports both RC-enabled and legacy services** - You can provision any service
in the IBM Cloud catalog.

* **Bindings Managment** - Automatically creates secrets with the credentials required to bind to
any provisioned service.

* **Bind Only Mode** - You do not need to provision a service to bind to it. You can bind to 
existing services.

## Installating the operator

The operator can be installed any Kubernetes cluster with version >= 1.11. You need to have an 
[IBM Cloud account](https://cloud.ibm.com/registration) and the 
[IBM Cloud CLI](https://cloud.ibm.com/docs/cli?topic=cloud-cli-getting-started).
You need alsp to have the [kubectl CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl/)  
already configured to access your cluster.

Before installing the operator, you need to login to IBM cloud with the IBM Cloud CLI:
```
ibmcloud login
```

and set a target environment for your resources with the command:
```
ibmcloud target --cf
```

To install the operator, run the following script:

```
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/install-operator.sh | bash 
```
This will install the latest version of the operator.

## Removing the operator

```
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/uninstall-operator.sh | bash 
```

## Using the Operator
TBD


## Troubleshooting

To find the current git revision for the operator, type:

```
kubectl exec -n ibmcloud-operators $(kubectl get pod -l "app=ibmcloud-operator" -n ibmcloud-operators -o jsonpath='{.items[0].metadata.name}') -- cat git-rev
```

## Learn more about how to contribute

- [contributions](./CONTRIBUTING.md)
