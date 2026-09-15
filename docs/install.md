# IBM Cloud Operator installation

Helm is the supported installation method for IBM Cloud Operator 2.0.0 and later. The OperatorHub catalog entry and the legacy `curl` installer are deprecated.

## Prerequisites

- Kubernetes 1.16 or later
- Helm 3 or later
- An IBM Cloud API key with access to the services the operator will manage

## Install the operator

Install the released chart from the IBM GitHub Container Registry namespace:

```bash
helm install ibmcloud-operator \
  oci://ghcr.io/ibm/ibmcloud-operator \
  --version 2.0.0 \
  --namespace ibmcloud-operator-system \
  --create-namespace
```

To install a chart from a source checkout instead:

```bash
helm install ibmcloud-operator ./charts/ibmcloud-operator \
  --namespace ibmcloud-operator-system \
  --create-namespace
```

The chart installs the `Service` and `Binding` CRDs, cluster-wide controller RBAC, the controller Deployment, and its metrics Service. To create a `ServiceMonitor`, set `metrics.serviceMonitor.enabled=true`. This requires the Prometheus Operator CRDs to be installed first.

## Configure an IBM Cloud account

Create the account Secret and defaults ConfigMap in every namespace where `Service` or `Binding` resources will be created. Do not put API keys in Helm values because Helm stores release values in the cluster.

```bash
export IBMCLOUD_API_KEY='<api-key>'
export IBMCLOUD_REGION='us-south'
export IBMCLOUD_RESOURCE_GROUP_ID='<resource-group-id>'
export WORKLOAD_NAMESPACE='<namespace>'

kubectl create namespace "$WORKLOAD_NAMESPACE"

kubectl -n "$WORKLOAD_NAMESPACE" create secret generic ibmcloud-operator-secret \
  --from-literal=api-key="$IBMCLOUD_API_KEY" \
  --from-literal=region="$IBMCLOUD_REGION"

kubectl -n "$WORKLOAD_NAMESPACE" create configmap ibmcloud-operator-defaults \
  --from-literal=region="$IBMCLOUD_REGION" \
  --from-literal=resourcegroupid="$IBMCLOUD_RESOURCE_GROUP_ID"
```

Cloud Foundry services also require `org` and `space` entries in `ibmcloud-operator-defaults`. See [Account context in operator secret and configmap](../README.md#account-context-in-operator-secret-and-configmap) for all supported values and management-namespace configuration.

Bindings resolve their IBM Cloud account context from the Binding namespace, including when `spec.serviceNamespace` refers to a Service in another namespace. This differs from releases before 2.0.0, which resolved it from the Service namespace.

## Upgrade

```bash
helm upgrade ibmcloud-operator \
  oci://ghcr.io/ibm/ibmcloud-operator \
  --version 2.0.0 \
  --namespace ibmcloud-operator-system
```

Before upgrading an installation that has cross-namespace Bindings, verify that the Binding namespace's IBM Cloud identity can manage each referenced service instance. Keys created by an older release used the Service namespace's identity and might need to be removed manually if the new identity cannot access them.

Helm installs CRDs on initial installation but does not upgrade or delete CRDs. Apply changed CRDs from the new chart before an upgrade when release notes require it.

## Migrate from OperatorHub

1. Stop the OperatorHub-managed controller by deleting its Subscription and ClusterServiceVersion without deleting the IBM Cloud Operator CRDs or custom resources.
2. Confirm that no old controller pod is running.
3. Install the Helm chart into `ibmcloud-operator-system`.
4. Verify that the Helm-managed controller reconciles existing resources.

Do not run the OperatorHub-managed and Helm-managed controllers at the same time.

## Uninstall

```bash
helm uninstall ibmcloud-operator --namespace ibmcloud-operator-system
```

Helm leaves the CRDs and all `Service` and `Binding` resources in place. Delete the CRDs only when you intend to delete all corresponding custom resources:

```bash
kubectl delete crd bindings.ibmcloud.ibm.com services.ibmcloud.ibm.com
```
