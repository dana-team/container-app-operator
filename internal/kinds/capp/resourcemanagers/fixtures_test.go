package resourcemanagers

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	cappName      = "my-capp"
	cappNamespace = "my-ns"

	elasticHost        = "https://elastic.example:9200/_bulk"
	elasticIndex       = "my-index"
	unsupportedLogType = "splunk"

	dnsZone      = "capp-zone.com."
	dnsCNAME     = "ingress.capp-zone.com."
	dnsProvider  = "dns-default"
	issuerName   = "letsencrypt-prod"
	issuerKind   = "ClusterIssuer"
	issuerGroup  = "cert-manager.io"
	hostnameBare = "my-app"
	hostnameFQDN = "my-app.capp-zone.com"

	ordersSource    = "orders"
	ordersA         = "orders-a"
	ordersB         = "orders-b"
	bootstrapServer = "kafka.example:9092"
	topicOrders     = "orders"
	topicPayments   = "payments"

	schedule = "* * * * *"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	return s
}

func newBaseCapp() cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
			UID:       types.UID("test-uid"),
		},
	}
}

func newCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.CappConfigName,
			Namespace: utils.CappNS,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			AutoscaleConfig: cappv1alpha1.AutoscaleConfig{
				RPS:             200,
				CPU:             80,
				Memory:          70,
				Concurrency:     10,
				ActivationScale: 3,
			},
		},
	}
}

func newSyslogNGScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(loggingv1beta1.AddToScheme(s))
	return s
}

func newLogSpec(logType cappv1alpha1.LogType) cappv1alpha1.LogSpec {
	spec := cappv1alpha1.LogSpec{
		Type:           logType,
		Host:           elasticHost,
		User:           "elastic-user",
		PasswordSecret: "elastic-creds",
	}
	if logType == cappv1alpha1.LogTypeElastic {
		spec.Index = elasticIndex
	}
	return spec
}

func newCappConfigWithDNS() *cappv1alpha1.CappConfig {
	cfg := newCappConfig()
	cfg.Spec.DNSConfig = cappv1alpha1.DNSConfig{
		Zone:     dnsZone,
		CNAME:    dnsCNAME,
		Provider: dnsProvider,
		IssuerRef: cappv1alpha1.IssuerRef{
			Name:  issuerName,
			Kind:  issuerKind,
			Group: issuerGroup,
		},
	}
	return cfg
}

func newCappWithHostname(hostname string) cappv1alpha1.Capp {
	capp := newBaseCapp()
	capp.Spec.RouteSpec.Hostname = hostname
	return capp
}

func newCappWithTLS(hostname string, tls bool) cappv1alpha1.Capp {
	capp := newCappWithHostname(hostname)
	capp.Spec.RouteSpec.TlsEnabled = tls
	return capp
}

func newSecret(name string, mutate func(*corev1.Secret)) *corev1.Secret {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
		},
	}
	if mutate != nil {
		mutate(sec)
	}
	return sec
}

func newFakeClient(scheme *runtime.Scheme, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}

func newKafkaSourceConfiguration() cappv1alpha1.KafkaSourceConfiguration {
	return cappv1alpha1.KafkaSourceConfiguration{
		BootstrapServers: []string{bootstrapServer},
		Topics:           []string{topicOrders, topicPayments},
		SecretRef:        corev1.LocalObjectReference{Name: "kafka-creds"},
	}
}

func newKafkaSourceEntry(name string, cfg cappv1alpha1.KafkaSourceConfiguration) cappv1alpha1.SourceConfiguration {
	return cappv1alpha1.SourceConfiguration{
		Name:                     name,
		KafkaSourceConfiguration: &cfg,
	}
}

func newPingSourceEntry(name string, cfg cappv1alpha1.PingSourceConfiguration) cappv1alpha1.SourceConfiguration {
	return cappv1alpha1.SourceConfiguration{
		Name:                    name,
		PingSourceConfiguration: &cfg,
	}
}
