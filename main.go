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

package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/ibm/cloud-operators/controllers"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	"github.com/ibm/cloud-operators/internal/ibmcloud/auth"
	"github.com/ibm/cloud-operators/internal/ibmcloud/cfservice"
	"github.com/ibm/cloud-operators/internal/ibmcloud/iam"
	"github.com/ibm/cloud-operators/internal/ibmcloud/resource"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ibmcloudv1alpha1 "github.com/ibm/cloud-operators/api/v1alpha1"
	ibmcloudv1beta1 "github.com/ibm/cloud-operators/api/v1beta1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = ibmcloudv1alpha1.AddToScheme(scheme)
	_ = ibmcloudv1beta1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "7c16769a.ibm.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.BindingReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Binding"),
		Scheme: mgr.GetScheme(),

		CreateCFServiceKey:         cfservice.CreateKey,
		CreateResourceServiceKey:   resource.CreateKey,
		DeleteCFServiceKey:         cfservice.DeleteKey,
		DeleteResourceServiceKey:   resource.DeleteKey,
		GetCFServiceKeyCredentials: cfservice.GetKey,
		GetIBMCloudInfo:            ibmcloud.GetInfo,
		GetResourceServiceKey:      resource.GetKey,
		GetServiceName:             resource.GetServiceName,
		GetServiceRoleCRN:          iam.GetServiceRoleCRN,
		SetControllerReference:     controllerutil.SetControllerReference,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Binding")
		os.Exit(1)
	}
	if err = (&controllers.ServiceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Service"),
		Scheme: mgr.GetScheme(),

		CreateCFServiceInstance:         cfservice.CreateInstance,
		CreateResourceServiceInstance:   resource.CreateServiceInstance,
		DeleteResourceServiceInstance:   resource.DeleteServiceInstance,
		GetCFServiceInstance:            cfservice.GetInstance,
		GetIBMCloudInfo:                 ibmcloud.GetInfo,
		GetResourceServiceAliasInstance: resource.GetServiceAliasInstance,
		GetResourceServiceInstanceState: resource.GetServiceInstanceState,
		UpdateResourceServiceInstance:   resource.UpdateServiceInstance,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
	}
	if err = (&controllers.TokenReconciler{
		Client:       mgr.GetClient(),
		Log:          ctrl.Log.WithName("controllers").WithName("Token"),
		Scheme:       mgr.GetScheme(),
		Authenticate: auth.New(http.DefaultClient),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Token")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
