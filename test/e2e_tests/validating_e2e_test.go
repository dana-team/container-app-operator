package e2e_tests

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	mock "github.com/dana-team/container-app-operator/test/e2e_tests/mocks"
	"github.com/dana-team/container-app-operator/test/e2e_tests/testconsts"
	utilst "github.com/dana-team/container-app-operator/test/e2e_tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

const (
	validDomainSuffix    = ".com"
	unsupportedHostname  = "...aaa.a...."
	clusterLocalHostname = "invalid.svc.cluster.local"
	invalidHostName      = "invalid_domain!"
	existingHostname     = "google.com"
	unsupportedLogType   = "unsupported"
	elasticLogType       = "elastic"
	elasticUser          = "user"
	elasticHostExample   = "https://elasticsearch.dana.com/_bulk"
	index                = "main"
	secretName           = "elastic-secret"
)

var _ = Describe("Validate the validating webhook", func() {
	It("Should deny the use of an invalid hostname", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.RouteSpec.Hostname = unsupportedHostname
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

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

	It("Should deny the use of a hostname not matching the allowed patterns", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.RouteSpec.Hostname = invalidHostName
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

	It("Should deny the use of an invalid log type", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.LogSpec.Type = unsupportedLogType
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

	It("Should deny the use of an incomplete log spec", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.LogSpec.Type = elasticLogType
		baseCapp.Spec.LogSpec.Host = elasticHostExample
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

	It("Should allow the use of a complete and supported log spec", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.LogSpec.Type = elasticLogType
		baseCapp.Spec.LogSpec.Host = elasticHostExample
		baseCapp.Spec.LogSpec.Index = index
		baseCapp.Spec.LogSpec.User = elasticUser
		baseCapp.Spec.LogSpec.PasswordSecret = secretName
		Expect(k8sClient.Create(context.Background(), baseCapp)).Should(Succeed())
	})

	It("Should deny a Capp with sources but without 'external' scale metric", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.ScaleMetric = testconsts.CPUScaleMetric
		baseCapp.Spec.Sources = []cappv1alpha1.KedaSource{
			{
				Name:       "test-source",
				ScalarType: "prometheus",
			},
		}
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

	It("Should deny a Capp with 'external' scale metric but without sources", func() {
		baseCapp := mock.CreateBaseCapp()
		baseCapp.Name = utilst.GenerateUniqueCappName(baseCapp.Name)
		baseCapp.Spec.ScaleMetric = testconsts.ExternalScaleMetric
		baseCapp.Spec.Sources = []cappv1alpha1.KedaSource{}
		Expect(k8sClient.Create(context.Background(), baseCapp)).ShouldNot(Succeed())
	})

})
