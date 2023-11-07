package test

import (
	"bytes"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// https://github.com/aws/amazon-vpc-cni-k8s/blob/7eeb2a9ab437887f77de30a5eab20bb42742df06/test/framework/resources/k8s/resources/pod.go#L188
func (env *Framework) PodExec(pod corev1.Pod, cmd string) (string, string, error) {
	restClient, err := env.getRestClientForPod(pod.Namespace, pod.Name)
	if err != nil {
		return "", "", err
	}
	command := []string{
		"sh",
		"-c",
		cmd,
	}
	execOptions := &corev1.PodExecOptions{
		Stdout:  true,
		Stderr:  true,
		Command: command,
	}

	restClient.Get()
	req := restClient.Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(execOptions, runtime.NewParameterCodec(scheme.Scheme))

	exec, err := remotecommand.NewSPDYExecutor(env.controllerRuntimeConfig, http.MethodPost, req.URL())
	if err != nil {
		return "", "", err
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(env.ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	// uncomment to see full output during test runs
	//gwlog.FallbackLogger.Debugf("PodExec stdout: %s", stdout.String())
	//gwlog.FallbackLogger.Debugf("PodExec stderr: %s", stderr.String())
	//gwlog.FallbackLogger.Debugf("PodExec err: %s", err)

	return stdout.String(), stderr.String(), err
}

func (env *Framework) GetPodsByDeploymentName(deploymentName string, deploymentNamespce string) []corev1.Pod {
	deployment := appsv1.Deployment{}
	env.Get(env.ctx, types.NamespacedName{Name: deploymentName, Namespace: deploymentNamespce}, &deployment)
	pods := &corev1.PodList{}
	env.Log.Infoln("deployment.Spec.Selector.MatchLabels:", deployment.Spec.Selector.MatchLabels)
	env.List(env.ctx, pods, client.MatchingLabelsSelector{
		Selector: labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels),
	})
	return pods.Items
}

func (env *Framework) getRestClientForPod(namespace string, name string) (rest.Interface, error) {
	pod := &corev1.Pod{}
	err := env.Get(env.ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, pod)
	if err != nil {
		return nil, err
	}

	gkv, err := apiutil.GVKForObject(pod, env.k8sScheme)
	if err != nil {
		return nil, err
	}
	return apiutil.RESTClientForGVK(gkv, false, env.controllerRuntimeConfig, serializer.NewCodecFactory(env.k8sScheme))
}
