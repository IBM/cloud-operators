package main

import (
	"io/ioutil"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
)

type Deployment struct {
	Name string                `json:"name"`
	Spec appsv1.DeploymentSpec `json:"spec"`
}

func getDeployments(output string) ([]Deployment, error) {
	var deployment appsv1.Deployment
	deploymentBytes, err := ioutil.ReadFile(filepath.Join(output, "apps_v1_deployment_ibmcloud-operators-controller-manager.yaml"))
	if err != nil {
		return nil, errors.Wrap(err, "Error reading generated deployment file. Did kustomize run yet?")
	}
	err = yaml.Unmarshal(deploymentBytes, &deployment)
	return []Deployment{
		{Name: deployment.Name, Spec: deployment.Spec},
	}, err
}
