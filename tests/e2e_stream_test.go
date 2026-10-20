package test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/helm"
	http_helper "github.com/gruntwork-io/terratest/modules/http-helper"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestE2EStreamBlackhole reproduces the failure class the stream probers
// exist for: an established connection whose packets are silently dropped
// while new connections keep working. Toxiproxy sits between the websocket
// prober and the astrolavos /echo peer; its "timeout" toxic with timeout=0
// holds the sockets open and forwards nothing, which is exactly a black
// hole. The prober must notice through a heartbeat timeout, count a
// reconnect, and recover once the toxic is removed.
func TestE2EStreamBlackhole(t *testing.T) {
	helmChartPath := "../deploy/kubernetes"
	kubectlOptions := k8s.NewKubectlOptions("", "", "default")
	releaseName := fmt.Sprintf("astrolavos-%s", strings.ToLower(random.UniqueId()))

	// toxiproxy: control API on 8474, proxy on 8000 (created below)
	k8s.KubectlApply(t, kubectlOptions, "testdata/toxiproxy.yaml")
	defer k8s.KubectlDelete(t, kubectlOptions, "testdata/toxiproxy.yaml")
	k8s.WaitUntilDeploymentAvailable(t, kubectlOptions, "toxiproxy", 30, 5*time.Second)

	// astrolavos probes its own /echo, but through toxiproxy so the test can
	// black-hole the flow. interval 1s / timeout 2s keep the test short.
	values := imageValues()
	values["fullnameOverride"] = releaseName
	values["autoscaling.enabled"] = "false"
	values["serviceMonitor.enabled"] = "false"
	values["config.endpoints"] = "- domain: toxiproxy:8000/echo\n  prober: websocket\n  interval: 1s\n  timeout: 2s\n  tag: e2e"

	options := &helm.Options{
		SetValues: values,
		ExtraArgs: map[string][]string{"install": {"--install"}},
	}

	defer helm.Delete(t, options, releaseName, true)
	helm.Upgrade(t, options, helmChartPath, releaseName)

	pods := k8s.ListPods(t, kubectlOptions, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	podName := pods[0].Name
	k8s.WaitUntilPodAvailable(t, kubectlOptions, podName, 15, 5*time.Second)

	api := k8s.NewTunnel(kubectlOptions, k8s.ResourceTypeService, "toxiproxy", 0, 8474)
	defer api.Close()
	api.ForwardPort(t)

	toxi := toxiproxyClient{base: "http://" + api.Endpoint()}
	toxi.do(t, http.MethodPost, "/proxies", fmt.Sprintf(
		`{"name":"echo","listen":"0.0.0.0:8000","upstream":"%s:80","enabled":true}`, releaseName))

	metrics := k8s.NewTunnel(kubectlOptions, k8s.ResourceTypePod, podName, 0, 3000)
	defer metrics.Close()
	metrics.ForwardPort(t)

	metricsURL := fmt.Sprintf("http://%s/metrics", metrics.Endpoint())
	series := `domain="ws://toxiproxy:8000/echo"`

	// 1. the stream is up: the connection ages
	waitForMetric(t, metricsURL, "astrolavos_stream_connection_age_seconds", series, func(v float64) bool { return v > 0 })

	// 2. black-hole both directions: sockets stay open, nothing is forwarded
	toxi.do(t, http.MethodPost, "/proxies/echo/toxics",
		`{"name":"blackhole_down","type":"timeout","stream":"downstream","toxicity":1.0,"attributes":{"timeout":0}}`)
	toxi.do(t, http.MethodPost, "/proxies/echo/toxics",
		`{"name":"blackhole_up","type":"timeout","stream":"upstream","toxicity":1.0,"attributes":{"timeout":0}}`)

	// 3. the heartbeat times out and the prober counts a reconnect
	waitForMetric(t, metricsURL, "astrolavos_stream_reconnects_total", series+`,prober_type="websocket",reason="timeout"`,
		func(v float64) bool { return v >= 1 })
	waitForMetric(t, metricsURL, "astrolavos_errors_total", `conn_reused="true",`+series+`,error="timeout"`,
		func(v float64) bool { return v >= 1 })

	// 4. remove the black hole: the prober reconnects and the connection ages again
	toxi.do(t, http.MethodDelete, "/proxies/echo/toxics/blackhole_down", "")
	toxi.do(t, http.MethodDelete, "/proxies/echo/toxics/blackhole_up", "")
	waitForMetric(t, metricsURL, "astrolavos_stream_connection_age_seconds", series, func(v float64) bool { return v > 0 })
}

// imageValues returns the chart image overrides from the environment so the
// suite can run against a locally built image (kind load docker-image) as
// well as a published release.
func imageValues() map[string]string {
	v := map[string]string{"image.tag": imageTag()}

	if r := getenv("IMAGE_REGISTRY"); r != "" {
		v["image.registry"] = r
	}

	if r := getenv("IMAGE_REPOSITORY"); r != "" {
		v["image.repository"] = r
	}

	if p := getenv("IMAGE_PULL_POLICY"); p != "" {
		v["image.pullPolicy"] = p
	}

	return v
}

// waitForMetric polls the exposition endpoint until the first sample of
// name whose label set contains labels satisfies ok.
func waitForMetric(t *testing.T, url, name, labels string, ok func(float64) bool) {
	t.Helper()

	http_helper.HttpGetWithRetryWithCustomValidation(t, url, nil, 40, 3*time.Second,
		func(status int, body string) bool {
			if status != http.StatusOK {
				return false
			}

			v, found := metricValue(body, name, labels)

			return found && ok(v)
		})
}

// metricValue finds `name{...labels...} value` in Prometheus text exposition.
func metricValue(body, name, labels string) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, name+"{") || !strings.Contains(line, labels) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}

		return v, true
	}

	return 0, false
}

// toxiproxyClient talks to the toxiproxy control API through a tunnel.
type toxiproxyClient struct {
	base string
}

func (c toxiproxyClient) do(t *testing.T, method, path, body string) {
	t.Helper()

	req, err := http.NewRequest(method, c.base+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("toxiproxy %s %s: %v", method, path, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxy %s %s: %v", method, path, err)
	}

	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("toxiproxy %s %s: %d %s", method, path, resp.StatusCode, out)
	}

	t.Logf("toxiproxy %s %s -> %d", method, path, resp.StatusCode)
}
