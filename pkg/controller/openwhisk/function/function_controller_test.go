package function

import (
	"log"
	"path/filepath"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ow "github.com/ibm/cloud-operators/pkg/controller/openwhisk/common"
	owpkg "github.com/ibm/cloud-operators/pkg/controller/openwhisk/pkg"
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

func TestFunction(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(30 * time.Second)

	RunSpecs(t, "Function Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))

	t = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "..", "config", "crds")},
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
	Expect(owpkg.Add(mgr)).NotTo(HaveOccurred())

	stop = owtest.StartTestManager(mgr)

	namespace = owtest.SetupKubeOrDie(cfg, "openwhisk-function-")
	scontext = context.New(c, reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: namespace}})

	clientset := owtest.GetClientsetOrDie(cfg)
	config := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "secretmessage",
		},
		Data: map[string][]byte{
			"verysecretkey": []byte("verysecretbody"),
		},
	}
	clientset.CoreV1().Secrets(namespace).Create(config)

	secret := owtest.LoadObject("testdata/secret-url.yaml", &v1.Secret{})
	clientset.CoreV1().Secrets(namespace).Create(secret.(*v1.Secret))

	owtest.ConfigureOwprops("seed-defaults-owprops", clientset.CoreV1().Secrets(namespace))
	owtest.ConfigureOwprops("seed-defaults-owprops2", clientset.CoreV1().Secrets(namespace))

	wskclient, err = ow.NewWskClient(scontext, nil)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	close(stop)
	t.Stop()
})

var _ = Describe("function", func() {

	DescribeTable("should be ready",
		func(specfile string, pkgspec string, expected string) {
			function := owtest.LoadFunction("testdata/" + specfile)
			obj := owtest.PostInNs(scontext, &function, true, 0)

			if pkgspec != "" {
				pkg := owtest.LoadPackage("testdata/" + pkgspec)
				owtest.PostInNs(scontext, &pkg, true, 0)
			}

			Eventually(owtest.GetState(scontext, obj)).Should(Equal(resv1.ResourceStateOnline))
			Eventually(owtest.GetAction(wskclient, function.Name)).ShouldNot(BeNil())

			Expect(owtest.InvokeAction(wskclient, function.Name, nil)).Should(MatchJSON(expected))
		},

		Entry("string param", "owf-echo-string.yaml", "", `{"data":"Paris"}`),
		Entry("object param", "owf-echo-object.yaml", "", `{"data":{"name": "John"}}`),
		Entry("boolean true param", "owf-echo-true.yaml", "", `{"data":true}`),
		Entry("string true param", "owf-echo-string-true.yaml", "", `{"data":"true"}`),
		Entry("number param", "owf-echo-number.yaml", "", `{"data":-50}`),
		Entry("null param", "owf-echo-null.yaml", "", `{}`),
		Entry("empty param", "owf-echo-no-value.yaml", "", `{}`),
		Entry("from secret param", "owf-echo-secret.yaml", "", `{"data":"verysecretbody"}`),
		Entry("from secret with url param", "owf-echo-secret-url.yaml", "", `{"url":"https://kafka-admin-prod02.messagehub.services.us-south.bluemix.net:443"}`),

		Entry("sequence runtime", "owf-sequence.yaml", "", `{}`),

		Entry("native", "owf-native-bash.yaml", "", `{"message":"Hello"}`),

		Entry("target", "owf-echo-string-owprops2.yaml", "", `{"data":"Paris2"}`),

		Entry("in package", "owf-action-in-package.yaml", "owp-apackagename.yaml", `{"data":"Paris"}`),
	)

	DescribeTable("should fail",
		func(specfile string) {
			function := owtest.LoadFunction("testdata/" + specfile)
			obj := owtest.PostInNs(scontext, &function, true, 0)
			Eventually(owtest.GetState(scontext, obj)).Should(Equal(resv1.ResourceStateFailed))

		},
		Entry("missing code, codeURI or native", "owf-invalid-nocode-noURI.yaml"),
		Entry("code is empty", "owf-invalid-emptycode.yaml"),
		Entry("codeURI is empty", "owf-invalid-emptycodeURI.yaml"),
		Entry("code and codeURI mutually exclusive", "owf-invalid-code-codeURI.yaml"),
	)
})
