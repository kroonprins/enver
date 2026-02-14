package sources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type StatefulSetFetcher struct {
	processor WorkloadProcessor
}

func (f *StatefulSetFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
	namespace := source.GetNamespace()
	statefulSet, err := clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset %s/%s: %w", namespace, source.Name, err)
	}

	return f.processor.ProcessPodSpec(
		clientset,
		statefulSet.Spec.Template.Spec,
		source,
		source.Name,
		"StatefulSet",
		namespace,
		outputDirectory,
	)
}
