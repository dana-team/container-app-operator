package utils

import (
	"fmt"
	"regexp"
	"strconv"

	knativev1 "knative.dev/serving/pkg/apis/serving/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNextRevisionName generates the name for the next revision based on the current revision name.
func GetNextRevisionName(currentRevision string) string {
	re := regexp.MustCompile(`(\d+)$`)
	match := re.FindStringSubmatch(currentRevision)
	revisionNumber, _ := strconv.Atoi(match[1])
	nextRevisionNumber := revisionNumber + 1
	nextRevisionName := fmt.Sprintf("%s%05d", currentRevision[:len(currentRevision)-len(match[0])], nextRevisionNumber)
	return nextRevisionName
}

// GetKSVC fetches and returns an existing instance of a Knative Serving.
func GetKSVC(k8sClient client.Client, name string, namespace string) *knativev1.Service {
	ksvc := &knativev1.Service{}
	GetResource(k8sClient, ksvc, name, namespace)
	return ksvc
}

// GetRevision fetches and returns an existing instance of a Knative Revision.
func GetRevision(k8sClient client.Client, name string, namespace string) *knativev1.Revision {
	revision := &knativev1.Revision{}
	GetResource(k8sClient, revision, name, namespace)
	return revision
}
