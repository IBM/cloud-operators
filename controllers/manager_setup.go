package controllers

import (
	"net/http"

	"github.com/ibm/cloud-operators/internal/config"
	"github.com/ibm/cloud-operators/internal/ibmcloud"
	"github.com/ibm/cloud-operators/internal/ibmcloud/auth"
	"github.com/ibm/cloud-operators/internal/ibmcloud/cfservice"
	"github.com/ibm/cloud-operators/internal/ibmcloud/iam"
	"github.com/ibm/cloud-operators/internal/ibmcloud/resource"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Controllers passes back references to set up controllers for test mocking purposes
type Controllers struct {
	*BindingReconciler
	*ServiceReconciler
	*TokenReconciler
}

func SetUpControllers(mgr ctrl.Manager) (*Controllers, error) {
	return setUpControllers(mgr, setupWithManagerOrErr)
}

func setUpControllers(mgr ctrl.Manager, setup controllerSetUpFunc) (*Controllers, error) {
	c := setUpControllerDependencies(mgr)
	options := controller.Options{
		MaxConcurrentReconciles: config.Get().MaxConcurrentReconciles,
	}

	var err error
	setup(&err, c.BindingReconciler, mgr, options)
	setup(&err, c.ServiceReconciler, mgr, options)
	setup(&err, c.TokenReconciler, mgr, options)
	// +kubebuilder:scaffold:builder

	return c, errors.Wrap(err, "Unable to setup controller")
}

type controllerSetUpFunc func(err *error, r reconciler, mgr ctrl.Manager, options controller.Options)

type reconciler interface {
	SetupWithManager(mgr ctrl.Manager, options controller.Options) error
}

func setupWithManagerOrErr(err *error, r reconciler, mgr ctrl.Manager, options controller.Options) {
	if *err != nil {
		return
	}
	*err = r.SetupWithManager(mgr, options)
}

func setUpControllerDependencies(mgr ctrl.Manager) *Controllers {
	return &Controllers{
		BindingReconciler: &BindingReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("Binding"),
			scheme: mgr.GetScheme(),

			CreateCFServiceKey:         cfservice.CreateKey,
			CreateResourceServiceKey:   resource.CreateKey,
			DeleteCFServiceKey:         cfservice.DeleteKey,
			DeleteResourceServiceKey:   resource.DeleteKey,
			GetCFServiceKeyCredentials: cfservice.GetKey,
			GetIBMCloudInfo:            ibmcloud.GetInfo,
			GetResourceServiceKey:      resource.GetKey,
			GetServiceInstanceCRN:      resource.GetServiceInstanceCRN,
			GetServiceName:             resource.GetServiceName,
			GetServiceRoleCRN:          iam.GetServiceRoleCRN,
			SetControllerReference:     controllerutil.SetControllerReference,
			SetOwnerReference:          controllerutil.SetOwnerReference,
		},
		ServiceReconciler: &ServiceReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("Service"),
			scheme: mgr.GetScheme(),

			CreateCFServiceInstance:         cfservice.CreateInstance,
			CreateResourceServiceInstance:   resource.CreateServiceInstance,
			DeleteCFServiceInstance:         cfservice.DeleteInstance,
			DeleteResourceServiceInstance:   resource.DeleteServiceInstance,
			GetCFServiceInstance:            cfservice.GetInstance,
			GetIBMCloudInfo:                 ibmcloud.GetInfo,
			GetResourceServiceAliasInstance: resource.GetServiceAliasInstance,
			GetResourceServiceInstanceState: resource.GetServiceInstanceState,
			UpdateResourceServiceInstance:   resource.UpdateServiceInstance,
		},
		TokenReconciler: &TokenReconciler{
			Client:       mgr.GetClient(),
			Log:          ctrl.Log.WithName("controllers").WithName("Token"),
			Scheme:       mgr.GetScheme(),
			Authenticate: auth.New(http.DefaultClient),
		},
	}
}
