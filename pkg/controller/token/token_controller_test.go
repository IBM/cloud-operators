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

package token

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "dummyapikey"}}

const timeout = time.Second * 10

func TestReconcile(t *testing.T) {
	g := NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(HaveOccurred())
	c = mgr.GetClient()

	// Setup reconciler
	reconciler := newReconciler(mgr)
	reconciler.(*ReconcileToken).httpClient = makeClient()

	recFn, requests := SetupTestReconcile(reconciler)
	g.Expect(add(mgr, recFn)).NotTo(HaveOccurred())
	defer close(StartTestManager(mgr, g))

	// Create the secret object and expect the Reconcile
	instance := makeSecret("dummyapikey", "VExS246avaUT6MXZ56SH_I-AeWo_-JmW0u79Jd8LiBH")
	err = c.Create(context.TODO(), instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(HaveOccurred())

	defer c.Delete(context.TODO(), instance)

	g.Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
	g.Eventually(func() *corev1.Secret {
		var secret corev1.Secret
		c.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "dummyapikey-tokens"}, &secret)
		return &secret
	}).Should(WithTransform(getData, And(
		HaveKeyWithValue("access_token", []byte(" Bearer dummytoken")),
		HaveKey("refresh_token"),
		HaveKey("uaa_token"),
		HaveKey("uaa_refresh_token"))))
}

func getData(secret *corev1.Secret) map[string][]byte {
	if secret != nil {
		return secret.Data
	}
	return nil
}

func makeSecret(name string, apikey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"seed.ibm.com/ibmcloud-token": "apikey",
			},
		},
		Data: map[string][]byte{
			"api-key": []byte(apikey),
		},
	}
}

func makeClient() *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body := ioutil.NopCloser(bytes.NewReader([]byte(`{"access_token":"Bearer dummytoken"}`)))
		return &http.Response{
			StatusCode: 200,
			Body:       body,
		}, nil
	})}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (r roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return r(request)
}
