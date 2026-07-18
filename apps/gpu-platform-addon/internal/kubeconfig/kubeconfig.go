package kubeconfig

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func Load(path string) (*rest.Config, error) {
	if path == "" {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("load in-cluster configuration: %w", err)
		}
		return config, nil
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", path, err)
	}
	return config, nil
}
