package test

import (
	"bytes"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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
func (env *Framework) PodExec(namespace string, podName string, cmd string, printOutput bool) (string, string, error) {
	env.log.Infoln("PodExec() [namespace: %v] [podName: %v] [command: %v] \n", namespace, podName, cmd)
	restClient, err := env.getRestClientForPod(namespace, podName)
	if err != nil {
		return "", "", err
	}
	command := []string{
		"sh",
		"-c",
		cmd,
	}
	execOptions := &v1.PodExecOptions{
		Stdout:  true,
		Stderr:  true,
		Command: command,
	}

	restClient.Get()
	req := restClient.Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
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
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	if printOutput {
		env.log.Infoln("stdout: ", stdoutStr)
		env.log.Infoln("stderr: ", stderrStr)
		env.log.Infoln("err: ", err)
	}

	return stdoutStr, stderrStr, err
}

func (env *Framework) GetPodsByDeploymentName(deploymentName string, deploymentNamespce string) []v1.Pod {
	deployment := appsv1.Deployment{}
	env.Get(env.ctx, types.NamespacedName{Name: deploymentName, Namespace: deploymentNamespce}, &deployment)
	pods := &v1.PodList{}
	env.log.Infoln("deployment.Spec.Selector.MatchLabels:", deployment.Spec.Selector.MatchLabels)
	env.List(env.ctx, pods, client.MatchingLabelsSelector{
		Selector: labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels),
	})
	return pods.Items
}

func (env *Framework) getRestClientForPod(namespace string, name string) (rest.Interface, error) {
	pod := &v1.Pod{}
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
