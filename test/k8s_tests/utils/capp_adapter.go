package utils

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
const RandStrLength = 10

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func generateRandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// CreateCapp creates a new Capp instance with a unique name and returns it.
func CreateCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) *rcsv1alpha1.Capp {
	randString := generateRandomString(RandStrLength)
	cappName := capp.Name + "-" + randString
	newCapp := capp.DeepCopy()
	newCapp.Name = cappName
	Expect(k8sClient.Create(context.Background(), newCapp)).To(Succeed())
	return newCapp

}

// UpdateCapp updates an existing Capp instance.
func UpdateCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) {
	Expect(k8sClient.Update(context.Background(), capp)).To(Succeed())
}

// DeleteCapp deletes an existing Capp instance.
func DeleteCapp(k8sClient client.Client, capp *rcsv1alpha1.Capp) {
	Expect(k8sClient.Delete(context.Background(), capp)).To(Succeed())

}

// GetCapp fetch existing and return an instance of Capp.
func GetCapp(k8sClient client.Client, name string, namespace string) *rcsv1alpha1.Capp {
	capp := &rcsv1alpha1.Capp{
		Spec: rcsv1alpha1.CappSpec{
			RouteSpec: rcsv1alpha1.RouteSpec{},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capp",
			Namespace: "test-ns",
		},
	}
	Eventually(func() error {
		return k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, capp)
	}, 16*time.Second, 2*time.Second).Should(Succeed(), "Should fetch capp")
	return capp
}
