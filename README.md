[![Build Status](https://travis-ci.com/IBM/cloud-operators.svg?branch=master)](https://travis-ci.com/IBM/cloud-operators)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/cloud-operators)](https://goreportcard.com/report/github.com/IBM/cloud-operators)
[![GoDoc](https://godoc.org/github.com/IBM/cloud-operators?status.svg)](https://godoc.org/github.com/IBM/cloud-operators)

# IBM Cloud Operator

<!-- SHOW operator hub -->

With the IBM Cloud Operator, you can provision and bind [IBM public cloud services](https://cloud.ibm.com/catalog#services) to your Kubernetes cluster in a Kubernetes-native way. The IBM Cloud Operator is based on the [Kubernetes custom resource definition (CRD) API](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) so that your applications can create, update, and delete IBM Cloud services from within the cluster by calling Kubnernetes APIs, instead of needing to use several IBM Cloud APIs in addition to configuring your app for Kubernetes.

For more information, see the [IBM Cloud Operator user guide](docs/user-guide.md).

<!-- END SHOW operator hub -->

## Table of content
*   [Features](#features)
*   [Prerequisites](#prerequisites)
*   [Setting up the operator](#setting-up-the-operator)
    *   [Using a service ID](#using-a-service-id)
    *   [Installing the operator for OpenShift clusters](#installing-the-operator-for-openshift-clusters)
    *   [Installing the operator for Kubernetes clusters](#installing-the-operator-for-kubernetes-clusters)
    *   [Uninstalling the operator](#uninstalling-the-operator)
*   [Using the IBM Cloud Operator](#using-the-ibm-cloud-operator)
*   [Using separate IBM Cloud accounts](#using-separate-ibm-cloud-accounts)
*   [Using a management namespace](#using-a-management-namespace)
*   [Reference documentation](#reference-documentation)
    * [Service properties](#service-properties)
    * [Binding properties](#binding-properties)
    * [Account context in operator secret and configmap](#account-context-in-operator-secret-and-configmap)
    * [Versions](#versions)
*   [Contributing](#contributing)
*   [Troubleshooting](#troubleshooting)

<!-- SHOW operator hub -->

## Features

* **Service provisioning**: Create any service with any plan that is available in the [IBM Cloud catalog](https://cloud.ibm.com/catalog#services).

* **Bindings management**: Automatically create Kubernetes secrets in your Kubernetes cluster with the credentials to bind to any provisioned service to the cluster.

[Back to top](#ibm-cloud-operator)

## Prerequisites

1.  Have an [IBM Cloud account](https://cloud.ibm.com/registration).
2.  Have a cluster that runs Kubernetes version 1.11 or later (OpenShift 3.11 or later).
3.  Install the required command line tools.
    *   [IBM Cloud CLI](https://cloud.ibm.com/docs/cli?topic=cloud-cli-getting-started) (`ibmcloud`)
    *   [Kubernetes CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (`kubectl`)
4.  Log in to your IBM Cloud account from the CLI.
    ```bash
    ibmcloud login
    ```
5.  Target the appropriate environment (`--cf`) and resource group (`-g`). Note that you must still set the Cloud Foundry `org` and `space` environment, even if you do not create any Cloud Foundry services.
    ```bash
    ibmcloud target --cf -g <default>
    ```
6.  Set the Kubernetes context of your CLI to your cluster so that you can run `kubectl` commands. For example, if your cluster is in IBM Cloud Kubernetes Service, run the following command.
    ```bash
    ibmcloud ks cluster config -c <cluster_name_or_ID>
    ```

    To check that your Kubernetes context is set to your cluster, run the following command.
    ```bash
    kubectl config current-context
    ```

[Back to top](#ibm-cloud-operator)

## Setting up the operator

You can use an installation script to set up the IBM Cloud Operator. The installation script stores an API key in a Kubernetes secret in your cluster that can be accessed by the IBM Cloud Operator. Next, the script sets default values that are used to provision IBM Cloud services, like the resource group and region to provision the services in. Later, you can override any default value in the `Service` custom resource. Finally, the script deploys the operator in your cluster in the `ibmcloud-operators` namespace.

Prefer to create the secrets and defaults yourself? See the [IBM Cloud Operator installation guide](docs/install.md).

### Using a service ID

By default, the installation script creates an IBM Cloud API key that impersonates your user credentials, to use to set up the IBM Cloud Operator. However, you might want to create a service ID in IBM Cloud Identity and Access Managment (IAM). By using a service ID, you can control access for the IBM Cloud Operator without having the permissions tied to a particular user, such as if that user leaves the company. For more information, see the [IBM Cloud docs](https://cloud.ibm.com/docs/account?topic=account-serviceids).

1.  Create a service ID in IBM Cloud IAM. 
    ```bash
    ibmcloud iam service-id-create serviceid-ico -d service-ID-for-ibm-cloud-operator
    ```
2.  Assign the service ID access to the required permissions.<!--what are these? maybe just administrator? or should they scope to a different role, different service, region, resource group, etc?-->
    ```bash
    ibmcloud iam service-policy-create serviceid-ico -r Administrator
    ```
3.  Create an API key for the service ID.
    ```bash
    ibmcloud iam service-api-key-create apikey-ico serviceid-ico -d api-key-for-ibm-cloud-operator
    ```
4.  Set the API key of the service ID as your CLI environment variable. Now, when you run the installation script, the script uses the service ID's API key. The following command is an example for macOS.
    ```bash
    setenv IBMCLOUD_API_KEY <apikey-ico-value>
    ```
5.  Confirm that the API key environment variable is set in your CLI.
    ```bash
    echo $IBMCLOUD_API_KEY
    ```
6.  Follow the [prerequisite steps](#prerequisites) to log in to the IBM Cloud account that owns the service ID.

[Back to top](#ibm-cloud-operator)

### Installing the operator for OpenShift clusters

Before you begin, complete the [prerequisite steps](#prerequisites) to log in to IBM Cloud and your cluster, and optionally set up a [service ID API key](#using-a-service-id).

To install the latest release for OpenShift before install, run the following script:

*   **Latest release**: 
    ```bash
    curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash
    ```

*   **Specific release**: Replace `-v 0.0.0` with the specific version that you want to install.
    ```bash
    curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- -v 0.0.0 store-creds
    ```

[Back to top](#ibm-cloud-operator)

<!-- END SHOW operator hub -->

### Installing the operator for Kubernetes clusters

Before you begin, complete the [prerequisite steps](#prerequisites) to log in to IBM Cloud and your cluster, and optionally set up a [service ID API key](#using-a-service-id).

*   **Stable release**: To install the latest stable release of the operator, run the following script. The stable release supports the `v1alpha1` Kubernetes API version.
    ```bash
    curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- -v 0.1.11 install
    ```

*   **Latest release**: The latest `v0.2.x` version involves major changes and is undergoing further testing to ensure a smoother transition. The latest release supports the `v1alpha1` or `v1beta1` Kubernetes API versions.
    ```bash
    curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh |bash -s -- install
    ```

*   **Specific release**: Replace `-v 0.0.0` with the specific version that you want to install.
    ```bash
    curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh |bash -s -- -v 0.0.0 install
    ```

[Back to top](#ibm-cloud-operator)

### Uninstalling the operator

Before you begin, complete the [prerequisite steps](#prerequisites) to log in to IBM Cloud and your cluster.

To remove the operator, run the following script:

```bash
curl -sL https://raw.githubusercontent.com/IBM/cloud-operators/master/hack/configure-operator.sh | bash -s -- remove
```

[Back to top](#ibm-cloud-operator)

<!-- SHOW operator hub -->

## Using the IBM Cloud Operator

To use the IBM Cloud Operator, create a service instance and then bind the service to your cluster. For more information, see the [sample configuration files](config/samples) and [user guide](docs/user-guide.md).

**Step 1: Creating a service instance**

1.  To create an instance of an IBM public cloud service, first create a `Service` custom resource file. For more options, see the [Service properties](#service-properties) reference doc.
    *   `<SERVICE_CLASS>` is the IBM Cloud service that you want to create. To list IBM Cloud services, run `ibmcloud catalog service-marketplace` and use the **Name** value from the output.
    *   `<PLAN>` is the plan for the IBM Cloud service that you want to create, such as `free` or `standard`. To list available plans, run `ibmcloud catalog service <SERVICE_CLASS> | grep plan`.

    ```yaml
    apiVersion: ibmcloud.ibm.com/v1beta1
    kind: Service
    metadata:
        name: myservice
    spec:
        plan: <PLAN>
        serviceClass: <SERVICE_CLASS>
    ```

2.  Create the service instance in your cluster.
    ```bash
    kubectl apply -f filepath/myservice.yaml
    ```
3.  Check that your service status is **Online** in your cluster.
    ```bash
    kubectl get services.ibmcloud
    NAME           STATUS   AGE
    myservice      Online   12s
    ```
4.  Verify that your service instance is created in IBM Cloud.
    ```bash
    ibmcloud resource service-instances | grep myservice
    ```

**Step 2: Binding the service instance**

1.  To bind your service to the cluster so that your apps can use the service, create a `Binding` custom resource, where the `serviceName` field is the name of the `Service` custom resource that you previously created. For more options, see [Binding properties](#binding-properties).

    ```yaml
    apiVersion: ibmcloud.ibm.com/v1beta1
    kind: Binding
    metadata:
        name: mybinding
    spec:
        serviceName: myservice
    ```    

2.  Create the binding in your cluster.
    ```bash
    kubectl apply -f filepath/mybinding.yaml
    ```
3.  Check that your service status is **Online**.
    ```bash
    kubectl get bindings.ibmcloud 
    NAME                 STATUS   AGE
    mybinding            Online   25s
    ```
4.  Check that a secret of the same name as your binding is created. The secret contains the service credentials that apps in your cluster can use to access the service.
    ```bash
    kubectl get secrets
    NAME                       TYPE                                  DATA   AGE
    mybinding                  Opaque                                6      102s
    ```

[Back to top](#ibm-cloud-operator)

<!-- END SHOW operator hub -->

## Using separate IBM Cloud accounts

You can provision IBM Cloud services in separate IBM Cloud accounts from the same cluster. To use separate accounts, update the secrets and configmap in the Kubernetes namespace where you want to create services and bindings. 

**Tip**: Just want to use a different account one time and don't want to manage a bunch of namespaces? You can also specify a different account in the individual [service configuration](#service-properties), by overriding the default [account context](#account-context-in-operator-secret-and-configmap).

1.  Get the IBM Cloud account details, including account ID, Cloud Foundry org and space, resource group, region, and API key credentials.
2.  Edit or replace the `secret-ibm-cloud-operator` secret in the Kubernetes namespace that you want to use to create services in the account.
3.  Edit or replace the `config-ibm-cloud-operator` configmap in the Kubernetes namespace that you want to use to create services in the account.
4.  Optional: [Set up a management namespace](#setting-up-a-management-namespace) so that cluster users with access across namespaces cannot see the API keys for the different IBM Cloud accounts. 

[Back to top](#ibm-cloud-operator)

## Setting up a management namespace

By default, the API key credentials and other IBM Cloud account information are stored in a secret and a configmap within each namespace where you create IBM Cloud Operator service and binding custom resources. However, you might want to hide access to this information from cluster users in the namespace. For example, you might have multiple IBM Cloud accounts that you do not want cluster users in different namespaces to know about.

1.  Create a management namespace that is named `safe`.
    ```bash
    kubectl create namespace safe
    ```
2.  In the namespace where the IBM Cloud Operator runs, create an `ibm-cloud-operator` configmap that points to the `safe` namespace.
    ```bash
    cat <<EOF | kubectl apply -f -
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: ibm-cloud-operator
      namespace: ibmcloud-operators # Or update to the namespace where IBM Cloud Operator is running
      labels:
        app.kubernetes.io/name: ibmcloud-operator
    data:
      namespace: safe
    EOF
    ```
3.  Copy the existing or create your own `secret-ibm-cloud-operator` secrets and `config-ibm-cloud-operator` configmaps from the other Kubernetes namespace into the `safe` namespace. **Important**: You must rename the secrets and configmaps with the following naming convention. **Tip**: You can create these new secrets and configmaps similarly to `make install`, if you are familiar with that process.
    ```
    <namespace>-secret-ibm-cloud-operator
    <namespace>-config-ibm-cloud-operator
    ```

    For example, if you have a cluster with three namespaces `default`, `test`, and `prod`:
    ```
    default-secret-ibm-cloud-operator
    default-config-ibm-cloud-operator
    test-secret-ibm-cloud-operator
    test-config-ibm-cloud-operator
    prod-secret-ibm-cloud-operator
    prod-config-ibm-cloud-operator
    ```
4.  Delete the `secret-ibm-cloud-operator` secrets and `config-ibm-cloud-operator` configmaps in the other Kubernetes namespaces. 

[Back to top](#ibm-cloud-operator)

## Reference documentation

### Service Properties

A `Service` custom resource includes the properties in the following table. Each `parameter` value is treated as a raw value by the operator and passed as-is. For more information, see the [IBM Cloud Operator user guide](docs/user-guide.md).

| Parameter            | Required | Type       | Comments                                                                                                   |
|:-----------------|:---------|:-----------|:-----------------------------------------------------------------------------------------------------------|
| serviceClass `*`     | Yes      | `string`   | The IBM Cloud service that you want to create. To list IBM Cloud services, run `ibmcloud catalog service-marketplace` and use the **Name** value from the output.|
| plan `*`              | Yes      | `string`   | The plan to use for the service instance, such as `free` or `standard`. To use an existing service instance, set to `Alias`.  To list available plans, run `ibmcloud catalog service <SERVICE_CLASS> | grep plan`. |
| serviceClassType `*`  | CF only  | `string`   | Set to `CF` for Cloud Foundry services. Otherwise, omit this field. |
| externalName `*`      | No       | `string`   | The name for the service instance in IBM Cloud, such as in the console.|
| parameters       | No       | `[]Param`  | Parameters that are passed in to create the service instance. These parameters vary by service, and can be anything, such as a number, string, or object. |
| tags             | No       | `[]string` | The IBM Cloud [tag](https://cloud.ibm.com/docs/account?topic=account-tag) to assign the service instance, to help organize your cloud resources such as in the IBM Cloud console. |
| context          | No       | `Context`  | The IBM Cloud account context to use instead of the [default account context](#account-context-in-operator-secret-and-configmap).|

`*` **Note**: The `serviceClass`, `plan`, `serviceClassType`, and `externalName` parameters are immutable. After you set these parameters, you cannot later edit their values. If you do edit the values, the changes are overwritten back to the original values.

[Back to top](#ibm-cloud-operator)

### Binding Properties

A `Binding` custom resources includes the properties in the following table. For more information, see the [IBM Cloud Operator user guide](docs/user-guide.md).

| Field            | Required | Type     | Comments                                                                                              |
|:-----------------|:---------|:---------|:------------------------------------------------------------------------------------------------------|
| serviceName      | Yes      | `string` | The name of the `Service` resource that corresponds to the service instance on which to create credentials for the binding. |
| serviceNamespace | No       | `string` | The namespace of the `Service` resource.|
| alias            | No       | `string` | The name of credentials, if this binding links to existing credentials.<!--what does this mean?-->|
| secretName       | No       | `string` | The name of the `Secret` to be created. If you do not specify a value, the secret is given the same name as the binding.|
| role             | No       | `string` | The IBM Cloud IAM role to create the credentials to the service instance. Review the each service's documentation for a description of the roles. If you do not specify a role, the IAM `Manager` service access role is used. If the service does not support the `Manager` role, the first returned role from the service is used. |
| parameters       | No       | `[]Any`  | Parameters that are passed in to create the create the service credentials. These parameters vary by service, and can be anything, such as an integer, string, or object. |

[Back to top](#ibm-cloud-operator)

### Account context in operator secret and configmap

The IBM Cloud Operator needs an account context, which indicates the API key and the details of the IBM Cloud account to be used for creating services. The API key is stored in a `secret-ibm-cloud-operator` secret that is created when the IBM Cloud Operator is installed. Account details such as the account ID, Cloud Foundry org and space, resource group, and region are stored in a `config-ibm-cloud-operator` configmap.

When you create an IBM Cloud Operator service or binding resource, the operator checks the namespace that you create the resource in for the secret and configmap. If the the operator does not find the secret and configmap, it checks its own namespace for a configmap that points to a [management namespace](#setting-up-a-management-namespace). Then, the operator checks the management namespace for `<namespace>-secret-ibm-cloud-operator` secrets and `<namespace>-config-ibm-cloud-operator` configmaps. If no management namespace exists, the operator checks the `default` namespace for the `secret-ibm-cloud-operator` secret and `config-ibm-cloud-operator` configmap.

You can override the account context in the `Service` configuration file with the `context` field, as described in the following table. You might override the account context if you want to use a different IBM Cloud account to provision a service, but do not want to create separate secrets and configmaps for different namespaces.

| Field            | Required | Type     | Description |
|:-----------------|:---------|:---------|:------------|
| org              | No       | `string` | The Cloud Foundry org. To list orgs, run `ibmcloud account orgs`. |
| space            | No       | `string` | The Cloud Foundry space. To list spaces, run `ibmcloud account spaces`. |
| region           | No       | `string` | The IBM Cloud region. To list regions, run `ibmcloud regions`. |
| resourceGroup    | No       | `string` | The IBM Cloud resource group name. You must also include the `resourceGroupID`. To list resource groups, run `ibmcloud resource groups`. |
| resourceGroupID  | No       | `string` | The IBM Cloud resource group ID. You must also include the `resourceGroup`. To list resource groups, run `ibmcloud resource groups`. |
| resourceLocation | No       | `string` | The location of the resource.<!--what is this?--> |

[Back to top](#ibm-cloud-operator)

### Versions

Review the supported Kubernetes API versions for the following IBM Cloud Operator versions.

| Operator version | Kubernetes API version |
| --- | --- |
| `v0.2.x` | `v1beta1` or `v1alpha1` |
| `v0.1.x` | `v1alpha` |

[Back to top](#ibm-cloud-operator)

## Contributing to the project

See [Contributing to Cloud Operators](./CONTRIBUTING.md)

[Back to top](#ibm-cloud-operator)

## Troubleshooting

See the [Troubleshooting guide](docs/troubleshooting.md).

[Back to top](#ibm-cloud-operator)
