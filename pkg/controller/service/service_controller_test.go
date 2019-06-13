package service

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

	context "github.com/ibm/cloud-operators/pkg/context"
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

func TestService(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(30 * time.Second)

	RunSpecs(t, "Service Suite")
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

	stop = test.StartTestManager(mgr)

	namespace = test.SetupKubeOrDie(cfg, "ibmcloud-service-")
	scontext = context.New(c, reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: namespace}})

	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	close(stop)
	t.Stop()
})

var _ = Describe("service", func() {

	DescribeTable("should be ready",
		func(specfile string, pkgspec string, expected string) {
			service := test.LoadService("testdata/" + specfile)
			obj := test.PostInNs(scontext, &service, true, 0)

			Eventually(test.GetState(scontext, obj)).Should(Equal(resv1.ResourceStateOnline))

			//Eventually(test.GetAction(wskclient, function.Name)).ShouldNot(BeNil())

			//Expect(test.InvokeAction(wskclient, function.Name, nil)).Should(MatchJSON(expected))
		},

		Entry("string param", "translator.yaml", "", `{"data":"Paris"}`),
		//		Entry("object param", "owf-echo-object.yaml", "", `{"data":{"name": "John"}}`),

	)

	/*
		DescribeTable("should fail",
			func(specfile string) {
				service := test.LoadService("testdata/" + specfile)
				obj := test.PostInNs(scontext, &service, true, 0)
				Eventually(test.GetState(scontext, obj)).Should(Equal(resv1.ResourceStateFailed))

			},
			Entry("missing code, codeURI or native", "owf-invalid-nocode-noURI.yaml"),
		)
	*/
})
