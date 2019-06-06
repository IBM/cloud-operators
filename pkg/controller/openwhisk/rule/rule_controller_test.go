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

package rule

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
	owf "github.com/ibm/cloud-operators/pkg/controller/openwhisk/function"
	owt "github.com/ibm/cloud-operators/pkg/controller/openwhisk/trigger"
	owtest "github.com/ibm/cloud-operators/test"
)

var (
	c         client.Client
	cfg       *rest.Config
	namespace string
	scontext  context.Context
	wskclient *whisk.Client
	t         *envtest.Environment
	stop      chan struct{}
)

func TestRule(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(30 * time.Second)

	RunSpecs(t, "Rule Suite")
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
	Expect(owt.Add(mgr)).NotTo(HaveOccurred())
	Expect(owf.Add(mgr)).NotTo(HaveOccurred())

	stop = owtest.StartTestManager(mgr)

	// Initialize objects
	namespace = owtest.SetupKubeOrDie(cfg, "openwhisk-rule-")
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
	function owv1.Function
	trigger  owv1.Trigger
	rule     owv1.Rule
}

var _ = Describe("rule", func() {

	DescribeTable("should be ready",
		func(specfile, fnfile, tgfile string) {
			function := owtest.LoadFunction("testdata/" + fnfile)
			trigger := owtest.LoadTrigger("testdata/" + tgfile)
			rule := owtest.LoadRule("testdata/" + specfile)

			fn := owtest.PostInNs(scontext, &function, true, 0)
			owtest.PostInNs(scontext, &trigger, true, 0)
			obj := owtest.PostInNs(scontext, &rule, false, 0)

			Eventually(owtest.GetState(scontext, obj)).Should(Equal(resv1.ResourceStateOnline))
			Eventually(owtest.GetState(scontext, fn)).Should(Equal(resv1.ResourceStateOnline))
		},
		Entry("location", "owr-hello-location.yaml", "owf-hello.yaml", "owt-location-update.yaml"),
	)
})
