package commands

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	argoapi "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	appclientset "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	"github.com/argoproj/argo-cd/v2/server"
	servercache "github.com/argoproj/argo-cd/v2/server/cache"
	cacheutil "github.com/argoproj/argo-cd/v2/util/cache"
	appstatecache "github.com/argoproj/argo-cd/v2/util/cache/appstate"
	"github.com/argoproj/argo-cd/v2/util/cli"
	"github.com/argoproj/argo-cd/v2/util/io"
	kubeutil "github.com/argoproj/argo-cd/v2/util/kube"
)

func initCommand(creator func(clientOpts *argoapi.ClientOptions) *cobra.Command) *cobra.Command {
	clientOpts := &argoapi.ClientOptions{}
	cmd := creator(clientOpts)
	ctx, cancel := context.WithCancel(context.Background())
	clientConfig := cli.AddKubectlFlagsToCmd(cmd)
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		ln, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			return err
		}
		port := ln.Addr().(*net.TCPAddr).Port
		io.Close(ln)

		restConfig, err := clientConfig.ClientConfig()
		if err != nil {
			return err
		}
		appClientset, err := appclientset.NewForConfig(restConfig)
		if err != nil {
			return err
		}
		kubeClientset, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return err
		}

		namespace, _, err := clientConfig.Namespace()
		if err != nil {
			return err
		}
		overrides := clientcmd.ConfigOverrides{}
		redisPort, err := kubeutil.PortForward("app.kubernetes.io/name=argocd-redis-ha-haproxy", 6379, namespace, &overrides)
		if err != nil {
			return err
		}

		redisClient := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("localhost:%d", redisPort)})
		appstateCache := appstatecache.NewCache(cacheutil.NewCache(cacheutil.NewRedisCache(redisClient, time.Hour)), time.Hour)

		srv := server.NewServer(ctx, server.ArgoCDServerOpts{
			EnableGZip:    false,
			Namespace:     namespace,
			ListenPort:    port,
			AppClientset:  appClientset,
			DisableAuth:   true,
			RedisClient:   redisClient,
			Cache:         servercache.NewCache(appstateCache, 0, 0, 0),
			KubeClientset: kubeClientset,
		})

		go srv.Run(ctx, port, 0)
		clientOpts.ServerAddr = fmt.Sprintf("localhost:%d", port)
		clientOpts.PlainText = true
		return nil
	}
	cmd.PostRun = func(cmd *cobra.Command, args []string) {
		cancel()
	}
	return cmd
}
