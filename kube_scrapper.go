package main

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"log"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Node struct {
	Name               string
	CPU                CPUUtilization
	Memory             MemoryUtilization
	StorageAllocatable StorageUtilization
	Pods               PodsStats
}

type PodsStats struct {
	AapJobs int
	UatJobs int
}

type StorageUtilization struct {
	Used       *resource.Quantity `json:"used,omitempty"`
	Available  *resource.Quantity `json:"available,omitempty"`
	Percentage float64            `json:"percentage,omitempty"`
}
type CPUUtilization struct {
	Used       int64   `json:"usedCores"`  // In milliCores (1/1000 of a core)
	Percentage float64 `json:"percentage"` // Percentage utilization
}

type MemoryUtilization struct {
	Used       *resource.Quantity `json:"used,omitempty"`
	Available  *resource.Quantity `json:"available,omitempty"`
	Percentage float64            `json:"percentage,omitempty"`
}

func printNodeUtilization(nodeUtilizations []Node) {
	for _, node := range nodeUtilizations {
		// Print the updated utilization information for each node
		fmt.Printf("Node Name: %s\n", node.Name)
		fmt.Printf("CPU Usage: %d milliCores / %d milliCores (%.2f%%)\n", node.CPU.Used, 1000, node.CPU.Percentage)

		// Memory check for nil before accessing Value
		if node.Memory.Used != nil && node.Memory.Available != nil {
			fmt.Printf("Memory Usage: %d bytes / %d bytes (%.2f%%)\n", node.Memory.Used.Value(), node.Memory.Available.Value(), node.Memory.Percentage)
		} else {
			fmt.Println("Memory Usage: Data not available")
		}

		// Storage check for nil before accessing Value
		if node.StorageAllocatable.Available != nil {
			fmt.Printf("Storage Free: %d bytes\n", node.StorageAllocatable.Available.Value())
		} else {
			fmt.Println("Storage Allocatable: Data not available")
		}

		fmt.Printf("Pods aap-jobs: %d\n", node.Pods.AapJobs)
		fmt.Printf("Pods uat-jobs: %d\n", node.Pods.UatJobs)

		fmt.Println("------------------------------------------------------")
	}
}

func get_pods(clientset *kubernetes.Clientset) {
	// Get pods in all namespaces
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), v1.ListOptions{})
	if err != nil {
		log.Fatalf("Error listing pods: %v", err)
	}

	// Print pod names
	for _, pod := range pods.Items {
		fmt.Printf("Pod name: %s\n", pod.GetName())
	}

}

func get_nodes(clientset *kubernetes.Clientset) {
	// Get pods in all namespaces
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), v1.ListOptions{})
	if err != nil {
		log.Fatalf("Error listing pods: %v", err)
	}

	// Print pod names
	for _, node := range nodes.Items {
		fmt.Printf("Node name: %s\n", node.GetName())
	}
}

func get_pods_on_node(clientset *kubernetes.Clientset, node string) {
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), v1.ListOptions{
		FieldSelector: "spec.nodeName=" + node,
	})
	if err != nil {
		log.Fatalf("Error listing pods: %v", err)
	}

	for _, pod := range pods.Items {
		fmt.Printf("Pod name: %s\n", pod.GetName())
	}
}

func getPodsOnNodeInNamespace(clientset *kubernetes.Clientset, node string, namespace string) int {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{
		FieldSelector: "spec.nodeName=" + node,
	})
	if err != nil {
		log.Fatalf("Error listing pods: %v", err)
	}

	return len(pods.Items)
}

func getNodesUtilization(clientset *kubernetes.Clientset, config *rest.Config) []Node {
	var nodesList []Node

	// Create a Metrics client
	metricsClient, err := metrics.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating metrics client: %v", err)
	}

	// Get the list of nodes
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), v1.ListOptions{})
	if err != nil {
		log.Fatalf("Error listing nodes: %v", err)
	}

	// Iterate through nodes and get metrics
	for _, node := range nodes.Items {
		// Fetch live metrics for the node
		nodeMetrics, err := metricsClient.MetricsV1beta1().NodeMetricses().Get(context.TODO(), node.Name, v1.GetOptions{})
		if err != nil {
			log.Printf("Error fetching metrics for node %s: %v", node.Name, err)
			continue
		}

		// Calculate CPU and Memory usage
		currentCPUUsage := nodeMetrics.Usage.Cpu().MilliValue()
		cpuAllocatable := node.Status.Allocatable.Cpu().MilliValue()

		currentMemoryUsage := nodeMetrics.Usage.Memory().MilliValue()
		memoryAllocatable := node.Status.Allocatable.Memory().MilliValue()

		storageUsed := nodeMetrics.Usage.StorageEphemeral().Value()
		storageAllocatable := node.Status.Allocatable.StorageEphemeral().Value()
		nodeUtilization := Node{
			Name: node.Name,
			CPU: CPUUtilization{
				Used:       currentCPUUsage,
				Percentage: float64(currentCPUUsage) / float64(cpuAllocatable) * 100,
			},
			Memory: MemoryUtilization{
				Used:       resource.NewQuantity(currentMemoryUsage, resource.DecimalSI),
				Available:  node.Status.Allocatable.Memory(),
				Percentage: float64(currentMemoryUsage) / float64(memoryAllocatable) * 100,
			},
			StorageAllocatable: StorageUtilization{
				Used:       resource.NewQuantity(storageUsed, resource.DecimalSI),
				Available:  resource.NewQuantity(storageAllocatable-storageUsed, resource.DecimalSI),
				Percentage: float64(storageUsed) / float64(storageAllocatable) * 100,
			},
			Pods: PodsStats{
				AapJobs: getPodsOnNodeInNamespace(clientset, node.Name, "aap-jobs"),
				UatJobs: getPodsOnNodeInNamespace(clientset, node.Name, "uat-jobs"),
			},
		}

		// Append to nodesList
		nodesList = append(nodesList, nodeUtilization)
	}

	return nodesList
}

func fetchNodeUtilizationPeriodically(clientset *kubernetes.Clientset, config *rest.Config, channel chan<- []Node) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Fetch the node utilization data
			nodesUtilization := getNodesUtilization(clientset, config)
			// Send the fetched data to the channel
			channel <- nodesUtilization
		}
	}
}
