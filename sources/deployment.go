package sources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DeploymentFetcher struct {
	processor WorkloadProcessor
}

func (f *DeploymentFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
	namespace := source.GetNamespace()
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, source.Name, err)
	}

	return f.processor.ProcessPodSpec(
		clientset,
		deployment.Spec.Template.Spec,
		source,
		source.Name,
		"Deployment",
		namespace,
		outputDirectory,
	)
}
