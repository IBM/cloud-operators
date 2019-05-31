
[![Build Status](https://travis.ibm.com/seed/cloud-operators.svg?token=bc6xtkRixk96zbXuAu7U&branch=master)](https://travis.ibm.com/seed/cloud-operators)

# IBM Cloud Operator
The IBM Cloud Operator provides a Kubernetes CRD-Based API to manage the lifecycle of IBM public cloud services.
This operator allows to provision and bind to any of the 130+ IBM public cloud services, and create and manage service specific resources, such as event streams topics, and cloud object store buckets.

## Supported Features

* **Supports both RC-enabled and legacy services** - You can provision any service
in the IBM Cloud catalog.

* **Bindings Managment** - Automatically creates secrets with the credentail to bind to
any provisioned service.

* **Bind Only Mode** - You do not need to provision a service to bind to it. You can bind to 
existing services.

## Installating the operator
TODO - change this whole section for public Github and remove pull secret

The operator can be installed any Kubernetes cluster enabled. You need to have an IBM Cloud account and the IBM Cloud CLI. 

Before installing the operator, you need to login to IBM cloud with the IBM Cloud CLI, set a target
environment with the `ibmcloud target` command.

First, [get a github token](https://github.ibm.com/settings/tokens) and set it in `IBM_GITHUB_TOKEN`. Then, run:

```
curl -sL https://${IBM_GITHUB_TOKEN}@raw.github.ibm.com/seed/cloud-operators/master/hack/install-operators.sh | bash 
```
This will install the latest version of the operator.

## Removing the operator

```
curl -sL https://${IBM_GITHUB_TOKEN}@raw.github.ibm.com/seed/cloud-operators/master/hack/uninstall-operators.sh | bash 
```

## Learn more about how to contribute

- [contributions](./CONTRIBUTING.md)
