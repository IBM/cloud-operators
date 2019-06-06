/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package invocation

import (
	"log"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
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

	"github.com/apache/incubator-openwhisk-client-go/whisk"

	context "github.com/ibm/cloud-operators/pkg/context"
	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	"github.com/ibm/cloud-operators/pkg/apis"
	owv1 "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
	owfn "github.com/ibm/cloud-operators/pkg/controller/openwhisk/function"
	owtest "github.com/ibm/cloud-operators/test"
)

var (
	c         client.Client
	cfg       *rest.Config
	namespace string
	scontext  context.Context
	wskclient *whisk.Client
	echoCode  = "const main = params => params || {}"

	t    *envtest.Environment
	stop chan struct{}
)

func TestInvocation(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(30 * time.Second)

	RunSpecs(t, "Invocation Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))

	// Start kube apiserver
	t = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "..", "config", "crds")},
		ControlPlaneStartTimeout: 2 * time.Minute,
	}
	apis.AddToScheme(scheme.Scheme)

	var err error
	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	Expect(err).NotTo(HaveOccurred())

	c = mgr.GetClient()

	// Add reconcilers
	recFn := newReconciler(mgr)
	Expect(add(mgr, recFn)).NotTo(HaveOccurred())
	Expect(owfn.Add(mgr)).NotTo(HaveOccurred()) // register function controller

	stop = owtest.StartTestManager(mgr)

	// Initialize objects
	namespace = owtest.SetupKubeOrDie(cfg, "openwhisk-invocation-")
	scontext = context.New(c, reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: namespace}})

	clientset := owtest.GetClientsetOrDie(cfg)
	owtest.ConfigureOwprops("seed-defaults-owprops", clientset.CoreV1().Secrets(namespace))

	wskclient, err = ow.NewWskClient(scontext, nil)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	close(stop)
	t.Stop()
})

type testCase struct {
	invocation owv1.Invocation
	function   *owv1.Function
	delay      time.Duration // delay before posting function
}

var _ = Describe("invocation", func() {

	DescribeTable("should be ready",
		func(specfile string, fnfile string, delay time.Duration) {
			var function owv1.Function
			if fnfile != "" {
				function = owtest.LoadFunction("testdata/" + fnfile)
				owtest.PostInNs(scontext, &function, true, delay)
			}
			invocation := owtest.LoadInvocation("testdata/" + specfile)
			obj := owtest.PostInNs(scontext, &invocation, false, 0)

			Eventually(owtest.GetState(scontext, obj)).Should(Equal(resv1.ResourceStateOnline))

			scontext.Client().Delete(scontext, obj)
			Eventually(owtest.GetObject(scontext, obj)).Should(BeNil())
			if invocation.Spec.Finalizer != nil {
				Expect(owtest.GetActivation(wskclient, invocation.Spec.Finalizer.Function)).ShouldNot(BeNil())
			}
		},

		Entry("with no args", "noargs.yaml", "", time.Duration(0)),
		Entry("with one arg", "in-hello.yaml", "", time.Duration(0)),
		Entry("with retries until no errors", "in-echo-error.yaml", "fn-echo-error.yaml", time.Duration(0)),
		Entry("with function delay", "in-hello-delay.yaml", "fn-echo-delay.yaml", 2*time.Second),

		Entry("with finalizer", "in-echo-finalizer.yaml", "fn-echo-finalizer.yaml", time.Duration(0)),

		Entry("with secret store", "in-echo-store-secret.yaml", "", time.Duration(0)),
		Entry("with secret store projection", "in-echo-store-secret-projection.yaml", "", time.Duration(0)),
	)

})
