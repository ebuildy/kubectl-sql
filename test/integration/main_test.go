//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	envKubeconfig string
	envNamespaces []string
	envDynClient  dynamic.Interface
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Fprintln(os.Stderr, "KUBEBUILDER_ASSETS is not set — run: export KUBEBUILDER_ASSETS=$(setup-envtest use --bin-path)")
		os.Exit(1)
	}

	env := &envtest.Environment{}

	cfg, err := env.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start envtest: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if stopErr := env.Stop(); stopErr != nil {
			fmt.Fprintf(os.Stderr, "failed to stop envtest: %v\n", stopErr)
		}
	}()

	// Write a kubeconfig to a temp file so the CLI binary can use it.
	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Clusters["envtest"] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: cfg.CAData,
	}
	kubeconfig.AuthInfos["envtest"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
	}
	kubeconfig.Contexts["envtest"] = &clientcmdapi.Context{
		Cluster:  "envtest",
		AuthInfo: "envtest",
	}
	kubeconfig.CurrentContext = "envtest"

	tmpFile, err := os.CreateTemp("", "envtest-kubeconfig-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp kubeconfig file: %v\n", err)
		os.Exit(1)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if err := clientcmd.WriteToFile(*kubeconfig, tmpFile.Name()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write kubeconfig: %v\n", err)
		os.Exit(1)
	}
	envKubeconfig = tmpFile.Name()

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create dynamic client: %v\n", err)
		os.Exit(1)
	}
	envDynClient = dynClient

	envNamespaces, err = SeedFixtures(context.Background(), dynClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to seed fixtures: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
