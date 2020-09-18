# Troubleshooting Guide

## Checking that the operator is correctly started

To check if the operator is correctly started, type:

```
kubectl get pods -n ibmcloud-operator-system
```

if the operator is running, you should get an output similar to the following:

```
NAME                                 READY   STATUS    RESTARTS   AGE
ibmcloud-operator-controller-manager-5885bd58c4-84q52   1/1     Running   0          7s
```

to check the operator logs, type:

```
kubectl logs -n ibmcloud-operator-system $(kubectl get pods -n ibmcloud-operator-system -o jsonpath='{.items[0].metadata.name}')
```

## Finding the current git revision for the operator

To find the current git revision for the operator, type:

```
kubectl exec -n ibmcloud-operator-system $(kubectl get pods -n ibmcloud-operator-system -o jsonpath='{.items[0].metadata.name}') -- cat git-rev
```
