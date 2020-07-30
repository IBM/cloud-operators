/*
 * Copyright 2020 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	// +kubebuilder:scaffold:imports
)

var (
	cfg           *rest.Config
	k8sClient     client.Client
	k8sManager    ctrl.Manager
	testEnv       *envtest.Environment
	testNameStem  string
	testNamespace string
)

func TestMain(m *testing.M) {
	exitCode := run(m)
	os.Exit(exitCode)
}

func run(m *testing.M) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		err := mainTeardown()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to tear down controller test suite:", err)
		}
	}()

	if err := mainSetup(ctx); err != nil {
		panic(err)
	}
	return m.Run()
}

func mainSetup(ctx context.Context) error {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	testNameStem = "ibmcloud-test-"
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		return err
	}

	err = ibmcloudv1beta1.AddToScheme(scheme.Scheme)
	if err != nil {
		return err
	}

	// +kubebuilder:scaffold:scheme

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
	if err != nil {
		return err
	}

	if err = (&BindingReconciler{
		Client: k8sManager.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Binding"),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager); err != nil {
		return errors.Wrap(err, "Failed to set up binding controller")
	}
	if err = (&ServiceReconciler{
		Client: k8sManager.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Service"),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager); err != nil {
		return errors.Wrap(err, "Failed to set up service controller")
	}
	/*
		if err = (&TokenReconciler{
			Client:     k8sManager.GetClient(),
			Log:        ctrl.Log.WithName("controllers").WithName("Token"),
			Scheme:     k8sManager.GetScheme(),
			HTTPClient: http.DefaultClient,
		}).SetupWithManager(k8sManager); err != nil {
			return errors.Wrap(err, "Failed to set up token controller")
		}
	*/

	go func() {
		err = k8sManager.Start(ctx.Done())
		if err != nil {
			panic("Failed to start manager: " + err.Error())
		}
	}()

	k8sClient = k8sManager.GetClient()

	testNamespace, err = mainSetupNamespace(ctx)
	if err != nil {
		return err
	}
	return setup()
}

func mainTeardown() error {
	return testEnv.Stop()
}

func mainSetupNamespace(ctx context.Context) (string, error) {
	ns := v1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: testNameStem}}
	err := k8sClient.Create(ctx, &ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}
	return ns.Name, nil
}
