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
	c := setUpControllerDependencies(mgr)

	options := controller.Options{
		MaxConcurrentReconciles: config.Get().MaxConcurrentReconciles,
	}
	if err := c.BindingReconciler.SetupWithManager(mgr, options); err != nil {
		return nil, errors.Wrap(err, "Unable to setup binding controller")
	}
	if err := c.ServiceReconciler.SetupWithManager(mgr, options); err != nil {
		return nil, errors.Wrap(err, "Unable to setup service controller")
	}
	if err := c.TokenReconciler.SetupWithManager(mgr, options); err != nil {
		return nil, errors.Wrap(err, "Unable to setup token controller")
	}

	// +kubebuilder:scaffold:builder
	return c, nil
}

func setUpControllerDependencies(mgr ctrl.Manager) *Controllers {
	return &Controllers{
		BindingReconciler: &BindingReconciler{
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
			GetServiceInstanceCRN:      resource.GetServiceInstanceCRN,
			GetServiceName:             resource.GetServiceName,
			GetServiceRoleCRN:          iam.GetServiceRoleCRN,
			SetControllerReference:     controllerutil.SetControllerReference,
		},
		ServiceReconciler: &ServiceReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("Service"),
			Scheme: mgr.GetScheme(),

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
