# Releasing IBM Cloud Operator

Version 2.0.0 and later are released as a container image and an OCI Helm chart. OperatorHub publishing is deprecated.

## Automated release

The `Release` GitHub Actions workflow runs when a GitHub release is published from a tag named `vMAJOR.MINOR.PATCH`. It:

1. Builds and pushes `cloudoperators/ibmcloud-operator:MAJOR.MINOR.PATCH` to Docker Hub.
2. Packages the chart with matching chart and application versions.
3. Pushes the chart to `oci://ghcr.io/ibm/ibmcloud-operator`.
4. Attaches the packaged chart to the GitHub release.

The repository must define `DOCKER_USERNAME` and `DOCKER_PASSWORD` Actions secrets. The workflow uses its `GITHUB_TOKEN` to publish the chart to GitHub Container Registry. After the first publication, verify that the package is linked to this repository and has public visibility so users can install it without registry credentials.

Run the release validation before creating the tag:

```bash
make validate-release RELEASE_VERSION=2.0.0
```

## Manual release

Authenticate Docker and Helm, then run:

```bash
export RELEASE_VERSION=2.0.0
export DOCKER_USERNAME='<docker-user>'
export DOCKER_PASSWORD='<docker-token>'
export HELM_REGISTRY_USERNAME='<github-user>'
export HELM_REGISTRY_PASSWORD='<github-token>'
make release
```

Override `IMG` or `HELM_REGISTRY` when publishing to another registry.

The legacy `release-operatorhub` and `validate-operatorhub-release` Make targets remain available only for maintaining existing version 1.x metadata.

## Retire the OperatorHub listings

A notice in this repository does not update the catalogs consumed by OperatorHub or OpenShift. After publishing 2.0.0, submit maintainer-authorized removal pull requests for `operators/ibmcloud-operator` in both catalog repositories:

- [k8s-operatorhub/community-operators](https://github.com/k8s-operatorhub/community-operators/tree/main/operators/ibmcloud-operator)
- [redhat-openshift-ecosystem/community-operators-prod](https://github.com/redhat-openshift-ecosystem/community-operators-prod/tree/main/operators/ibmcloud-operator)

The OpenShift package only targets historical OpenShift versions and is not present in current file-based catalogs. Do not add it to current catalogs solely to attach deprecation metadata. Removing the stale package sources is the established retirement path for operators that move to Helm.
