package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/client-go/machine/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// scaleUp increases the replica count of a MachineSet
func scaleUp(clientset *kubernetes.Clientset, k8sClient client.Client, machinesetName, namespace string) {
	// Retrieve the MachineSet
	machineset := &v1beta1.MachineSet{}
	err := k8sClient.Get(context.TODO(), client.ObjectKey{Name: machinesetName, Namespace: namespace}, machineset)
	if err != nil {
		log.Printf("Error retrieving MachineSet %s: %v", machinesetName, err)
		return
	}

	// Increase the replicas count
	if machineset.Spec.Replicas != nil {
		*machineset.Spec.Replicas = *machineset.Spec.Replicas + 1
		log.Printf("Scaling up MachineSet %s to %d replicas", machinesetName, *machineset.Spec.Replicas)
	} else {
		log.Printf("MachineSet %s has no replica count defined", machinesetName)
		return
	}

	// Update the MachineSet
	err = k8sClient.Update(context.TODO(), machineset)
	if err != nil {
		log.Printf("Error updating MachineSet %s: %v", machinesetName, err)
		return
	}

	log.Printf("Successfully scaled up MachineSet %s", machinesetName)
}

// markMachine annotates a Machine object for scaling down or deletion
func markMachine(clientset *versioned.Clientset, machineName, namespace string) bool {
	// Retrieve the machine object
	machineObj, err := clientset.MachineV1beta1().Machines(namespace).Get(context.TODO(), machineName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error retrieving machine: %v", err)
		return false
	}

	// Log existing annotations for debugging
	log.Printf("Existing annotations for machine %s: %v", machineName, machineObj.Annotations)

	// Add or update the annotation to the machine
	if machineObj.Annotations == nil {
		machineObj.Annotations = make(map[string]string)
	}
	machineObj.Annotations["machine.openshift.io/cluster-api-delete-machine"] = "true"

	// Update the machine with the modified annotation
	updatedMachineObj, err := clientset.MachineV1beta1().Machines(namespace).Update(context.TODO(), machineObj, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Error updating machine annotation: %v", err)
		return false
	}

	// Log updated annotations for verification
	log.Printf("Updated annotations for machine %s: %v", machineName, updatedMachineObj.Annotations)

	log.Printf("Machine %s has been marked with machine.openshift.io/cluster-api-delete-machine=true", machineName)
	return true
}

// scaleDown reduces the replica count of a MachineSet in OpenShift
func scaleDown(clientset *kubernetes.Clientset, machineClientSet *versioned.Clientset, k8sClient client.Client, machinesetName, namespace, machineName string) {
	// Mark the machine for scaling down
	if !markNode(clientset, machineName) {
		log.Printf("Failed to mark node %s, skipping scaling down MachineSet", machineName)
		return
	}
	time.Sleep(1 * time.Minute)
	//there should be waiting for workers
	if !markMachine(machineClientSet, machineName, namespace) {
		log.Printf("Failed to mark machine %s, skipping scaling down MachineSet", machineName)
		return
	}

	// Retrieve the MachineSet
	machineset := &v1beta1.MachineSet{}
	err := k8sClient.Get(context.TODO(), client.ObjectKey{Name: machinesetName, Namespace: namespace}, machineset)
	if err != nil {
		log.Printf("Error retrieving MachineSet %s: %v", machinesetName, err)
		return
	}

	// Reduce the replicas count if greater than zero
	if machineset.Spec.Replicas != nil && *machineset.Spec.Replicas > 0 {
		*machineset.Spec.Replicas = *machineset.Spec.Replicas - 1
		log.Printf("Scaling down MachineSet %s to %d replicas", machinesetName, *machineset.Spec.Replicas)
	} else {
		log.Printf("MachineSet %s is already at 0 replicas", machinesetName)
		return
	}

	// Update the MachineSet
	err = k8sClient.Update(context.TODO(), machineset)
	if err != nil {
		log.Printf("Error updating MachineSet %s: %v", machinesetName, err)
		return
	}

	log.Printf("Successfully scaled down MachineSet %s", machinesetName)
}

func markNode(clientset *kubernetes.Clientset, nodeName string) bool {
	// Retrieve the node object
	// Node name to cordon

	// Get the node object
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Failed to get node %s: %v", nodeName, err)
	}

	// Mark the node as unschedulable (cordon)
	node.Spec.Unschedulable = true

	// Update the node status
	_, err = clientset.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("Failed to update node %s: %v", nodeName, err)
	}

	fmt.Printf("Node %s has been cordoned (unschedulable).\n", nodeName)
	return true
}
