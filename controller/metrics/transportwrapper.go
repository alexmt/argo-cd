package metrics

import (
	"io"
	"net/http"
	"strconv"

	"github.com/argoproj/pkg/kubeclientmetrics"
	"k8s.io/client-go/rest"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// AddMetricsTransportWrapper adds a transport wrapper which increments 'argocd_app_k8s_request_total' counter on each kubernetes request
func AddMetricsTransportWrapper(server *MetricsServer, app *v1alpha1.Application, config *rest.Config) *rest.Config {
	inc := func(resourceInfo kubeclientmetrics.ResourceInfo) error {
		namespace := resourceInfo.Namespace
		kind := resourceInfo.Kind
		statusCode := strconv.Itoa(resourceInfo.StatusCode)
		server.IncKubernetesRequest(app, resourceInfo.Server, statusCode, string(resourceInfo.Verb), kind, namespace)
		return nil
	}

	newConfig := kubeclientmetrics.AddMetricsTransportWrapper(config, inc)
	return newConfig
}

type bodyWrapper struct {
	metricsServer *MetricsServer
	body          io.ReadCloser
	readCount     int64
	server        string
	kind          string
	verb          kubeclientmetrics.K8sRequestVerb
}

func (b *bodyWrapper) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	b.readCount += int64(n)
	return n, err
}

func (b *bodyWrapper) Close() error {
	if b.readCount > 0 {
		b.metricsServer.ObserveKubernetesResponseSize(b.server, b.readCount, string(b.verb), b.kind)
	}
	return b.body.Close()
}

func AddMetricsClusterTransportWrapper(server string, metricsServer *MetricsServer, config *rest.Config) *rest.Config {
	return kubeclientmetrics.AddMetricsTransportWrapperWithResp(config, func(info kubeclientmetrics.ResourceInfo, response *http.Response) error {
		if info.Kind != "" && response != nil && response.Body != nil {
			response.Body = &bodyWrapper{
				metricsServer: metricsServer,
				server:        server,
				kind:          info.Kind,
				verb:          info.Verb,
				body:          response.Body,
				readCount:     0,
			}
		}
		return nil
	})
}
