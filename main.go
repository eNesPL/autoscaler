package main

import (
	_ "context"
	"fmt"
	_ "fmt"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	versionedclient "github.com/openshift/client-go/machine/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"log"
	"os"
	_ "os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

/*
	IDEAS to implement

# This is proof of concept not solution
It has multiple places with hardcoded variables and even more not resolved problems with scaling
aap scrapping should be replaced with direct access  to DB for optimization
*/
type ScaleData struct {
	NodeUtilizations []Node
	PendingJobs      []Response
}

func NewConfigWithAPIAndToken(apiServer, token string) (*rest.Config, error) {
	return &rest.Config{
		Host:        apiServer, // Kubernetes API server endpoint
		BearerToken: token,     // Bearer token for authentication
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true, // Set to true if the Kubernetes API server uses a self-signed certificate
		},
	}, nil
}

func main() {
	// Replace these with your API server and token
	kube_apiServer := os.Getenv("KUBE_API_SERVER")
	kube_token := os.Getenv("KUBE_TOKEN")
	aap_url := os.Getenv("AAP_URL")
	aap_token := os.Getenv("AAP_TOKEN")
	instance_groups := os.Getenv("INSTANCE_GROUPS")

	// Create a Kubernetes kube_config using the provided API server and token
	kubeConfig, err := NewConfigWithAPIAndToken(kube_apiServer, kube_token)
	var scheme = runtime.NewScheme()
	// Rejestracja w scheme standardowych zasobów
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(fmt.Errorf("błąd rejestracji K8s w scheme: %w", err))
	}

	// Rejestracja MachineSet z OpenShift
	if err := machinev1.Install(scheme); err != nil {
		panic(fmt.Errorf("błąd rejestracji MachineSet: %w", err))
	}
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Error creating controller-runtime client: %v", err)
	}

	if err != nil {
		log.Fatalf("Error creating Kubernetes kube_config: %v", err)
	}

	// Create a clientset from the kube_config
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Error creating Kubernetes clientset: %v", err)
	}

	nodeUtilizationChannel := make(chan []Node)

	var aapConfig = aapconfig{
		url:            aap_url,
		token:          aap_token,
		instanceGroups: instance_groups,
	}

	ch := make(chan []Response, len(instance_groups))
	//test command to scale down
	versionedClientSet, err := versionedclient.NewForConfig(kubeConfig)
	// machine set name, and machine name are static so should be changed
	scaleDown(clientset, versionedClientSet, k8sClient, "cacfarodev-vtlqg-worker-eastus3", "openshift-machine-api", "cacfarodev-vtlqg-worker-eastus3-mpg8z")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		fetchNodeUtilizationPeriodically(clientset, kubeConfig, nodeUtilizationChannel)
	}()

	go func() {
		defer wg.Done()
		fetchPendingJobsPeriodically(aapConfig, ch)
	}()

	go func() {
		var latestNodeUtilizations []Node
		var latestPendingJobs []Response

		for {
			select {
			case nodeUtilization := <-nodeUtilizationChannel:
				latestNodeUtilizations = nodeUtilization
			case pendingJobs := <-ch:
				latestPendingJobs = pendingJobs
			}

			if latestNodeUtilizations != nil && latestPendingJobs != nil {
				scaleLogic(ScaleData{
					NodeUtilizations: latestNodeUtilizations,
					PendingJobs:      latestPendingJobs,
				})
			}
		}
	}()

	wg.Wait()
}

func scaleLogic(data ScaleData) {

	if data.NodeUtilizations == nil || data.PendingJobs == nil {
		return
	}
	//log.Printf("Scaling based on Node Utilizations: %+v", data.NodeUtilizations)
	//log.Printf("Scaling based on Pending Jobs: %+v", data.PendingJobs)
	printNodeUtilization(data.NodeUtilizations)
	printJob(data.PendingJobs)

}
