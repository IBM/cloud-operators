/*
 * Copyright 2019 IBM Corporation
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

package binding

import (
	goContext "context"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	context "github.com/ibm/cloud-operators/pkg/context"
	svctr "github.com/ibm/cloud-operators/pkg/controller/service"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	"github.com/ibm/cloud-operators/pkg/apis"
	test "github.com/ibm/cloud-operators/test"
)

var (
	c         client.Client
	cfg       *rest.Config
	namespace string
	scontext  context.Context
	t         *envtest.Environment
	stop      chan struct{}
)

func TestBinding(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(60 * time.Second)

	RunSpecs(t, "Binding Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))

	t = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "config", "crds")},
		ControlPlaneStartTimeout: 2 * time.Minute,
	}
	apis.AddToScheme(scheme.Scheme)

	var err error
	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	mgr, err := manager.New(cfg, manager.Options{})
	Expect(err).NotTo(HaveOccurred())

	c = mgr.GetClient()

	recFn := newReconciler(mgr)
	Expect(add(mgr, recFn)).NotTo(HaveOccurred())
	Expect(svctr.Add(mgr)).NotTo(HaveOccurred()) // add service controller

	stop = test.StartTestManager(mgr)

	namespace = test.SetupKubeOrDie(cfg, "ibmcloud-binding-")
	scontext = context.New(c, reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: namespace}})

})

var _ = AfterSuite(func() {
	close(stop)
	t.Stop()
})

var _ = Describe("binding", func() {

	DescribeTable("should be ready",
		func(servicefile string, bindingfile string) {
			service := test.LoadService("testdata/" + servicefile)
			svcobj := test.PostInNs(scontext, &service, true, 0)

			// make sure service is online
			Eventually(test.GetState(scontext, svcobj)).Should(Equal(resv1.ResourceStateOnline))

			// now test creation of binding
			binding := test.LoadBinding("testdata/" + bindingfile)
			bndobj := test.PostInNs(scontext, &binding, true, 0)

			// check binding is online
			Eventually(test.GetState(scontext, bndobj)).Should(Equal(resv1.ResourceStateOnline))

			// check secret is created
			clientset := test.GetClientsetOrDie(cfg)
			_, err := clientset.CoreV1().Secrets(namespace).Get(goContext.Background(), binding.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

		},

		// TODO - add more entries
		Entry("string param", "translator.yaml", "translator-binding.yaml"),
	)

	DescribeTable("should delete",
		func(servicefile string, bindingfile string) {
			svc := test.LoadService("testdata/" + servicefile)
			svc.Namespace = namespace
			bnd := test.LoadBinding("testdata/" + bindingfile)
			bnd.Namespace = namespace

			// delete binding
			test.DeleteObject(scontext, &bnd, true)

			// test secret is deleted
			clientset := test.GetClientsetOrDie(cfg)
			Eventually(isSecretDeleted(clientset, bnd.Name)).Should(Equal(true))

			// delete service & return when done
			test.DeleteObject(scontext, &svc, true)

			Eventually(test.GetObject(scontext, &svc)).Should((BeNil()))
		},

		// TODO - add more entries
		Entry("string param", "translator.yaml", "translator-binding.yaml"),
	)

})

// check if secret is deleted
func isSecretDeleted(clientset *kubernetes.Clientset, secretName string) func() bool {
	return func() bool {
		_, err := clientset.CoreV1().Secrets(namespace).Get(goContext.Background(), secretName, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			return true
		}
		return false
	}
}
