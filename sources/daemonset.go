package sources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DaemonSetFetcher struct {
	processor WorkloadProcessor
}

func (f *DaemonSetFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
	namespace := source.GetNamespace()
	daemonSet, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonset %s/%s: %w", namespace, source.Name, err)
	}

	return f.processor.ProcessPodSpec(
		clientset,
		daemonSet.Spec.Template.Spec,
		source,
		source.Name,
		"DaemonSet",
		namespace,
		outputDirectory,
	)
}
