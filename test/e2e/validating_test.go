package e2e

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e/mocks"
	utilst "github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

const (
	validDomainSuffix    = ".com"
	clusterLocalHostname = "invalid.svc.cluster.local"
	existingHostname     = "google.com"
)

var _ = Describe("Validate the validating webhook", func() {
	It("Should deny the use of an existing hostname", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.RouteSpec.Hostname = existingHostname
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

	It("Should deny the use of a hostname in cluster local", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.RouteSpec.Hostname = clusterLocalHostname
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

	It("Should allow the use of a hostname matching the allowed patterns", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.RouteSpec.Hostname = fmt.Sprintf("%s.allowed", baseCapp.Name)
		Expect(k8sClient.Create(context.Background(), baseCapp)).Should(Succeed())
	})

	It("Should allow the use of a unique and valid hostname", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		validHostName := baseCapp.Name + validDomainSuffix
		baseCapp.Spec.RouteSpec.Hostname = validHostName
		Expect(k8sClient.Create(context.Background(), baseCapp)).Should(Succeed())
	})

	It("Should allow updating a capp with a hostname that has not been changed", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		validHostName := baseCapp.Name + validDomainSuffix
		baseCapp.Spec.RouteSpec.Hostname = validHostName
		Expect(k8sClient.Create(context.Background(), baseCapp)).Should(Succeed())

		err := retry.RetryOnConflict(utilst.NewRetryOnConflictBackoff(), func() error {
			cappInCluster := cappv1alpha1.Capp{}
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: baseCapp.Name, Namespace: baseCapp.Namespace}, &cappInCluster); err != nil {
				return err
			}
			cappInCluster.Spec.RouteSpec.Hostname = validHostName
			return k8sClient.Update(context.Background(), &cappInCluster)
		})
		Expect(err).ToNot(HaveOccurred())
	})

})
