package utils

import (
	"fmt"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
)

func GetNextRevisionName(currentRevision string) string {
	re := regexp.MustCompile(`(\d+)$`)
	match := re.FindStringSubmatch(currentRevision)
	revisionNumber, _ := strconv.Atoi(match[1])
	nextRevisionNumber := revisionNumber + 1
	nextRevisionName := fmt.Sprintf("%s%05d", currentRevision[:len(currentRevision)-len(match[0])], nextRevisionNumber)
	return nextRevisionName
}

// GetKsvc f retrieves existing instance of Ksvc and returns it.
func GetKsvc(k8sClient client.Client, name string, namespace string) *knativev1.Service {
	ksvc := &knativev1.Service{}
	GetResource(k8sClient, ksvc, name, namespace)
	return ksvc
}

// GetRevision retrieves existing instance of Revision and returns it.
func GetRevision(k8sClient client.Client, name string, namespace string) *knativev1.Revision {
	revision := &knativev1.Revision{}
	GetResource(k8sClient, revision, name, namespace)
	return revision
}
