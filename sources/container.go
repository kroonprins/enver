package sources

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"enver/transformations"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type ContainerFetcher struct {
	restConfig *rest.Config
}

func NewContainerFetcher(restConfig *rest.Config) *ContainerFetcher {
	return &ContainerFetcher{restConfig: restConfig}
}

func (f *ContainerFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
	namespace := source.GetNamespace()

	// Validate kind
	validKinds := map[string]bool{
		"Pod":         true,
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}
	if source.Kind == "" {
		return nil, fmt.Errorf("kind is required for Container source %q", source.Name)
	}
	if !validKinds[source.Kind] {
		return nil, fmt.Errorf("invalid kind %q for Container source %q (must be Pod, Deployment, StatefulSet, or DaemonSet)", source.Kind, source.Name)
	}

	// Find the target pod
	var podName string
	var pod *corev1.Pod
	var err error

	switch source.Kind {
	case "Pod":
		podName = source.Name
		pod, err = clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, podName, err)
		}
	case "Deployment":
		pod, err = f.findPodForDeployment(clientset, namespace, source.Name)
		if err != nil {
			return nil, err
		}
		podName = pod.Name
	case "StatefulSet":
		pod, err = f.findPodForStatefulSet(clientset, namespace, source.Name)
		if err != nil {
			return nil, err
		}
		podName = pod.Name
	case "DaemonSet":
		pod, err = f.findPodForDaemonSet(clientset, namespace, source.Name)
		if err != nil {
			return nil, err
		}
		podName = pod.Name
	}

	// Check pod is running
	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("pod %s/%s is not running (phase: %s)", namespace, podName, pod.Status.Phase)
	}

	// Convert transformation configs
	var transformConfigs []transformations.Config
	for _, tc := range source.Transformations {
		transformConfigs = append(transformConfigs, transformations.Config{
			Type:          tc.Type,
			Target:        tc.Target,
			Value:         tc.Value,
			Variables:     tc.Variables,
			Output:        tc.Output,
			Key:           tc.Key,
			BaseDirectory: outputDirectory,
		})
	}

	// Build set of container names to include
	containerFilter := make(map[string]bool)
	for _, name := range source.Containers {
		containerFilter[name] = true
	}
	filterContainers := len(containerFilter) > 0

	var entries []EnvEntry

	// Process each container
	for _, container := range pod.Spec.Containers {
		// Skip if container is not in the filter list
		if filterContainers && !containerFilter[container.Name] {
			continue
		}

		// Exec into container and run env command
		envOutput, err := f.execEnvCommand(clientset, namespace, podName, container.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to exec into container %s in pod %s/%s: %w", container.Name, namespace, podName, err)
		}

		// Parse env output
		containerEntries, err := f.parseEnvOutput(envOutput, source, container.Name, podName, namespace, transformConfigs)
		if err != nil {
			return nil, err
		}

		entries = append(entries, containerEntries...)
	}

	return entries, nil
}

func (f *ContainerFetcher) findPodForDeployment(clientset *kubernetes.Clientset, namespace, deploymentName string) (*corev1.Pod, error) {
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, deploymentName, err)
	}

	// Get pods matching the deployment's selector
	labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	return f.findRunningPod(clientset, namespace, labelSelector, "Deployment", deploymentName)
}

func (f *ContainerFetcher) findPodForStatefulSet(clientset *kubernetes.Clientset, namespace, statefulSetName string) (*corev1.Pod, error) {
	statefulSet, err := clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), statefulSetName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset %s/%s: %w", namespace, statefulSetName, err)
	}

	// Get pods matching the statefulset's selector
	labelSelector := metav1.FormatLabelSelector(statefulSet.Spec.Selector)
	return f.findRunningPod(clientset, namespace, labelSelector, "StatefulSet", statefulSetName)
}

func (f *ContainerFetcher) findPodForDaemonSet(clientset *kubernetes.Clientset, namespace, daemonSetName string) (*corev1.Pod, error) {
	daemonSet, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), daemonSetName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonset %s/%s: %w", namespace, daemonSetName, err)
	}

	// Get pods matching the daemonset's selector
	labelSelector := metav1.FormatLabelSelector(daemonSet.Spec.Selector)
	return f.findRunningPod(clientset, namespace, labelSelector, "DaemonSet", daemonSetName)
}

func (f *ContainerFetcher) findRunningPod(clientset *kubernetes.Clientset, namespace, labelSelector, workloadType, workloadName string) (*corev1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for %s %s/%s: %w", workloadType, namespace, workloadName, err)
	}

	// Find the first running pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for %s %s/%s", workloadType, namespace, workloadName)
	}

	return nil, fmt.Errorf("no running pods found for %s %s/%s (found %d pods, none running)", workloadType, namespace, workloadName, len(pods.Items))
}

func (f *ContainerFetcher) execEnvCommand(clientset *kubernetes.Clientset, namespace, podName, containerName string) (string, error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   []string{"env"},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(f.restConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("exec failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

func (f *ContainerFetcher) parseEnvOutput(output string, source Source, containerName, podName, namespace string, transformConfigs []transformations.Config) ([]EnvEntry, error) {
	var entries []EnvEntry

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split on first = only (values can contain =)
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := line[:idx]
		value := line[idx+1:]

		if source.ShouldExcludeVariable(key) {
			continue
		}

		transformedKey, transformedValue, err := transformations.ApplyTransformations(key, value, transformConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to apply transformation: %w", err)
		}

		entries = append(entries, EnvEntry{
			Key:        transformedKey,
			Value:      transformedValue,
			SourceType: "Container",
			Name:       fmt.Sprintf("%s/%s", podName, containerName),
			Namespace:  namespace,
		})
	}

	return entries, nil
}
