package kube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps kubectl operations for a specific kubeconfig
type Client struct {
	Kubeconfig string
	UseSudo    bool
}

// NewDataPlaneClient creates a client for the Ryzen data plane
func NewDataPlaneClient(kubeconfig string) *Client {
	return &Client{Kubeconfig: kubeconfig, UseSudo: false}
}

// NewControlPlaneClient creates a client for the ThinkPad control plane
func NewControlPlaneClient() *Client {
	return &Client{UseSudo: true}
}

// Run executes a kubectl command and returns stdout
func (c *Client) Run(args ...string) (string, error) {
	cmdArgs := c.buildArgs(args...)
	var cmd *exec.Cmd
	if c.UseSudo {
		cmd = exec.Command("sudo", append([]string{"kubectl"}, cmdArgs...)...)
	} else {
		cmd = exec.Command("kubectl", cmdArgs...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl %s: %s (%w)", strings.Join(args, " "), stderr.String(), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunJSON executes kubectl with -o json and unmarshals result
func (c *Client) RunJSON(result interface{}, args ...string) error {
	args = append(args, "-o", "json")
	out, err := c.Run(args...)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(out), result)
}

// Apply applies a YAML manifest
func (c *Client) Apply(yaml string) (string, error) {
	cmdArgs := c.buildArgs("apply", "-f", "-")
	var cmd *exec.Cmd
	if c.UseSudo {
		cmd = exec.Command("sudo", append([]string{"kubectl"}, cmdArgs...)...)
	} else {
		cmd = exec.Command("kubectl", cmdArgs...)
	}
	cmd.Stdin = strings.NewReader(yaml)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl apply: %s (%w)", stderr.String(), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Patch patches a resource with JSON merge or strategic merge
func (c *Client) Patch(resource, name, namespace, patchType, patch string) (string, error) {
	args := []string{"patch", resource, name, "-n", namespace, "--type", patchType, "-p", patch}
	return c.Run(args...)
}

// Delete deletes a resource
func (c *Client) Delete(resource, name, namespace string) (string, error) {
	args := []string{"delete", resource, name, "-n", namespace, "--ignore-not-found"}
	return c.Run(args...)
}

// GetPodName finds the first pod matching a label in a namespace
func (c *Client) GetPodName(namespace, labelSelector string) (string, error) {
	out, err := c.Run("get", "pods", "-n", namespace, "-l", labelSelector,
		"-o", "jsonpath={.items[0].metadata.name}")
	if err != nil {
		return "", fmt.Errorf("no pod found with label %s in %s", labelSelector, namespace)
	}
	if out == "" {
		return "", fmt.Errorf("no pod found with label %s in %s", labelSelector, namespace)
	}
	return out, nil
}

// GetDeploymentName finds the deployment matching a label
func (c *Client) GetDeploymentName(namespace, labelSelector string) (string, error) {
	out, err := c.Run("get", "deploy", "-n", namespace, "-l", labelSelector,
		"-o", "jsonpath={.items[0].metadata.name}")
	if err != nil || out == "" {
		return "", fmt.Errorf("no deployment found with label %s in %s", labelSelector, namespace)
	}
	return out, nil
}

// Exec runs a command inside a pod
func (c *Client) Exec(namespace, pod string, command ...string) (string, error) {
	args := append([]string{"exec", "-n", namespace, pod, "--"}, command...)
	return c.Run(args...)
}

// RolloutRestart triggers a rolling restart of a deployment
func (c *Client) RolloutRestart(namespace, deployment string) (string, error) {
	return c.Run("rollout", "restart", "deployment", deployment, "-n", namespace)
}

// RolloutStatus waits for rollout to complete
func (c *Client) RolloutStatus(namespace, deployment string) (string, error) {
	return c.Run("rollout", "status", "deployment", deployment, "-n", namespace, "--timeout=60s")
}

// GetConfigMapValue reads a specific key from a ConfigMap
func (c *Client) GetConfigMapValue(namespace, cmName, key string) (string, error) {
	jsonpath := fmt.Sprintf("{.data.%s}", key)
	return c.Run("get", "configmap", cmName, "-n", namespace, "-o", "jsonpath="+jsonpath)
}

// PatchConfigMap patches a ConfigMap data field
func (c *Client) PatchConfigMap(namespace, cmName, key, value string) (string, error) {
	patch := fmt.Sprintf(`{"data":{%q:%q}}`, key, value)
	return c.Patch("configmap", cmName, namespace, "merge", patch)
}

// FindConfigMap finds a ConfigMap matching a prefix in a namespace
func (c *Client) FindConfigMap(namespace, prefix string) (string, error) {
	out, err := c.Run("get", "configmap", "-n", namespace, "-o", "name")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimPrefix(line, "configmap/")
		if strings.HasPrefix(name, prefix) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no ConfigMap with prefix %q in %s", prefix, namespace)
}

// buildArgs prepends kubeconfig flag if set
func (c *Client) buildArgs(args ...string) []string {
	if c.Kubeconfig != "" {
		return append([]string{"--kubeconfig=" + c.Kubeconfig}, args...)
	}
	return args
}
