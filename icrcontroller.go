package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *controller) makeActiveICRPod(pod *corev1.Pod) bool {
	ctx := context.Background()

	fmt.Println("### Making active the ICR POD: ", pod.Name)
	// Now change the stdby pod's role to active.
	pod.Labels["role"] = "active"

	for i := 0; i < 5; i++ {
		fmt.Println("Attempting to update the ICR POD to make it active. Attempt number: ", i)

		// Update the POD by calling "Update" to the apiserver
		_, err := c.clientset.CoreV1().Pods("default").Update(ctx, pod, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(i, "Error making ICR pod as active: ", pod.Name, "\n", err.Error())
			pod, err = c.clientset.CoreV1().Pods("default").Get(ctx, pod.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Println("Error getting POD while retrying updates: ", err.Error())
			} else {
				pod.Labels["role"] = "active"
			}
		} else {
			fmt.Println("Made ICR pod as active: ", pod.Name, " at time: ", time.Now())
			return true
		}
	}
	return true
}

func isICRPod(pod *corev1.Pod) bool {
	return pod.Labels["app"] == "icr"
}

func (c *controller) handleICRUpdate(pod *corev1.Pod) {
	// Nothing to do aas of now
}

func (c *controller) handleICRAdd(pod *corev1.Pod) {
	ctx := context.Background()

	if pod.Name == myPodName {
		fmt.Println("@@@ Ignoring ADD event for self")
		return
	}

	if am_I_active_icr_pod == true {
		fmt.Println("@@@ A new ICR pod has been added and I am already active. So nothing to do.")
		return
	}

	fmt.Println("\n\n @@@@@@ Handling ADD event for ICR pod: ", pod.Name, " in pod: ", myPodName)

	if pod.Labels["role"] == "active" {
		fmt.Println("There is an existing active ICR POD. So I will remain standby ICR POD.")
		return
	}

	fmt.Println("Comparing pods: ", myPodName, " and ", pod.Name)
	/* Earlier, I was trying to select the pod with earliest start time. But that was found to be the same for the pods.
	 * Hence, I am using the Pod Name to select one active from the 2 pods.
	 * Pod names are randomly assigned by Kubernetes.
	 */
	if strings.Compare(myPodName, pod.Name) < 0 {
		tmppod, err := c.clientset.CoreV1().Pods("default").Get(ctx, myPodName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("@@@ Error retrieving self ICR pod while going active: ", err.Error())
			return
		}
		fmt.Println("@@@@ Going active: ", myPodName)
		am_I_active_icr_pod = true
		c.makeActiveICRPod(tmppod)
	}
}

func (c *controller) handleICRDel(pod *corev1.Pod) {
	ctx := context.Background()

	fmt.Println("\n\n @@@@@@ Handling DEL event for ICR pod: ", pod.Name, " in pod: ", myPodName)

	if am_I_active_icr_pod == true {
		// Nothing to do if I am active ICR pod
		return
	}

	if pod.Name == myPodName {
		// I hope this never happens! I get my own deletion message...
		return
	}

	// I was standby ICR POD and I need to become active now.
	fmt.Println("@@@ Active icr pod gone. Going active: ", myPodName)

	tmppod, err := c.clientset.CoreV1().Pods("default").Get(ctx, myPodName, metav1.GetOptions{})
	if err != nil {
		fmt.Println("@@@ Error retrieving self ICR pod while going active: ", err.Error())
		return
	}
	fmt.Println("@@@@ Going active: ", myPodName)
	am_I_active_icr_pod = true
	c.makeActiveICRPod(tmppod)
	c.icr_post_failover_reconcile()
}
