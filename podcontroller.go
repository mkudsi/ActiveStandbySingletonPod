package main

import (
	"context"
	"fmt"
	"net"
	"time"

	//netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var activePodSelected bool = false
var currActivePodName string = "InvalidPod"

type controller struct {
	clientset      kubernetes.Interface
	podLister      appslisters.PodLister
	podCacheSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
}

func newController(clientset kubernetes.Interface, podInformer appsinformers.PodInformer) *controller {
	c := &controller{
		clientset:      clientset,
		podLister:      podInformer.Lister(),
		podCacheSynced: podInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ekspose"),
	}

	podInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleAdd,
			UpdateFunc: c.handleUpdate,
			DeleteFunc: c.handleDel,
		},
	)

	return c
}

func (c *controller) processItem() bool {

	// This is a blocking call. It will wait until anything is put in the queue.
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	defer c.queue.Forget(item)
	thispod := item.(*corev1.Pod)

	fmt.Println("\n\nProcessing POD: ", thispod.Name, " at : ", time.Now())

	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Printf("\nError getting key from cache %s\n", err.Error())
	}

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Printf("\nError splitting key into namespace and name %s\n", err.Error())
		return false
	}

	var actvpod corev1.Pod
	var stdbypod corev1.Pod

	var actvfound bool
	var stdbyfound bool
	ctx := context.Background()

	actvpod, stdbypod, actvfound, stdbyfound, _ = c.get_actv_stdby()

	// check if the object has been deleted from k8s cluster
	tmppod, err := c.clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fmt.Println("Handling delete event for POD: ", thispod.Name)

		// If the POD which just went down was active, then make the standby pod as active if present.
		if thispod.Labels["role"] == "active" {
			if actvfound == true && thispod.Name != actvpod.Name {
				fmt.Println("Error: Found another active when active went down. thispod: ", thispod.Name, " actvpod: ", actvpod.Name)
			}

			if thispod.Name != currActivePodName {
				fmt.Println("Error: Mismatch in expected active POD cache, thispod: ", thispod.Name, " currActvPod: ", currActivePodName)
			}

			activePodSelected = false
			currActivePodName = "InvalidPod"

			if stdbyfound == true {
				fmt.Println("@@@@ Found a standbypod after active went down: ", stdbypod.Name)
				tmppod, err = c.clientset.CoreV1().Pods(ns).Get(ctx, stdbypod.Name, metav1.GetOptions{})
				if err != nil {
					fmt.Println("Error: Failure fetching stdby pod freshly while trying to make it active")
				} else {

					rv := c.makeActive(tmppod)
					if rv == false {
						fmt.Println("Error: Could not make pod as active : ", stdbypod.Name)
					}
				}
			}

		} else {
			fmt.Println("Standby POD went down. It will be re-created by replicaset. Nothing to do here.")
		}
	} else {
		thispod = tmppod
		fmt.Println("Handling add event for POD: ", thispod.Name)
		if thispod.Labels["role"] == "active" { // If this pod being added is an active pod
			if actvfound == true && actvpod.Name != thispod.Name {
				fmt.Println("A new pod is created and there is already an active pod: ", actvpod.Name)
			}
			// To somehow resolve this mismatch...
		} else { // If this pod being added is a standby pod
			if activePodSelected == false {
				if actvfound == true && actvpod.Name != thispod.Name {
					fmt.Println("A standby pod is added and there is an existing active pod. So this will remain standby: ", thispod.Name)
				} else {
					activePodSelected = true
					currActivePodName = thispod.Name
					fmt.Println("A new POD is created as standby and there is no other active pod.",
						" Making this pod as active: ", thispod.Name)
					c.makeActive(thispod)
				}
			}
		}
	}

	fmt.Println("\nFinished: CurrActivePodName: ", currActivePodName, " and activePodSelect : ", activePodSelected)

	return true
}

func (c *controller) inform_standby_pod_to_become_active(pod *corev1.Pod) bool {

	serveraddr, err := net.ResolveUDPAddr("udp", pod.Status.PodIP+":10001")
	if err != nil {
		fmt.Println("Error resolveaddr : ", err.Error())
	}

	Conn, _ := net.DialUDP("udp", nil, serveraddr)

	defer Conn.Close()
	Conn.Write([]byte("GoActiveDuringFailover"))
	return true
}

func (c *controller) makeActive(pod *corev1.Pod) bool {
	ctx := context.Background()

	fmt.Println("### Making active the POD with IP : ", pod.Status.PodIP)
	// Now change the stdby pod's role to active.
	pod.Labels["role"] = "active"

	for i := 0; i < 5; i++ {
		fmt.Println("Attempting to update the POD to make it active. Update attempt number: ", i)

		// Update the POD by calling "Update" to the apiserver
		_, err := c.clientset.CoreV1().Pods("default").Update(ctx, pod, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(i, "Error making pod as active: ", pod.Name, "\n", err.Error())
			pod, err = c.clientset.CoreV1().Pods("default").Get(ctx, pod.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Println("Error getting POD while retrying updates: ", err.Error())
			} else {
				pod.Labels["role"] = "active"
			}
		} else {
			fmt.Println("Made pod as active: ", pod.Name, " at time: ", time.Now())
			currActivePodName = pod.Name
			activePodSelected = true
			c.inform_standby_pod_to_become_active(pod)
			return true
		}
	}
	return true
}

// This function returns an active and a standby pod from the list of pods and the total number of pods as well.
// There may be multiple standby pods detected. The oldest one will be returned in that case.
func (c *controller) get_actv_stdby() (actvpod corev1.Pod, stdbypod corev1.Pod,
	actvfound bool, stdbyfound bool, total_pods int) {
	ctx := context.Background()

	actvfound = false
	stdbyfound = false
	total_pods = 0

	podlist, err := c.clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{LabelSelector: "app=hapod"})
	if err != nil {
		fmt.Println("Error while retrieving pod list during delete %s\n", err.Error())
		return
	}

	var earliest_stdby_pod_time metav1.Time = metav1.Now()

	// This loop will find PODs with an active and a standby role label.
	for _, ppod := range podlist.Items {
		if ppod.Labels["role"] == "active" {
			fmt.Println("@@@ Pod ", ppod.Name, " is active")
			actvfound = true
			actvpod = ppod
		} else if ppod.Labels["role"] == "standby" {
			fmt.Println("@@@ Pod ", ppod.Name, " is standby and its  Start time is ", ppod.Status.StartTime)
			// A more recent pod may not really be ready yet and ip address may not yet have been allocated to it.
			// Aim here is to find out the oldest standby pod so that it can be made active when active pod goes down.
			if ppod.Status.StartTime.Before(&earliest_stdby_pod_time) {
				earliest_stdby_pod_time = *ppod.Status.StartTime
				stdbypod = ppod
				stdbyfound = true
			}
		} else {
			fmt.Println("@@@ Pod ", ppod.Name, " is not standby. Problem with pod definition file.")
		}
	}
	total_pods = len(podlist.Items)
	fmt.Println("Total number of pods is : ", total_pods, " activefound is ", actvfound, " stdby found is ", stdbyfound)
	return
}

func (c *controller) handleAdd(obj interface{}) {

	thispod := obj.(*corev1.Pod)
	fmt.Println("\n\nQueueing A new Pod that has been added : ", thispod.Name, " at ", time.Now())

	// Can handle the logic here directly instead of using queue.
	// Just experimenting with the queue to learn about it.
	c.queue.Add(obj)
}

func (c *controller) handleDel(obj interface{}) {
	thispod := obj.(*corev1.Pod)
	fmt.Println("\n\nQueueing A Pod that has been deleted : ", thispod.Name, " at ", time.Now())

	c.queue.Add(obj)
}

func (c *controller) killthepod(pod *corev1.Pod) bool {
	ctx := context.Background()

	fmt.Println("### Killing POD : ", pod.Name)
	//fmt.Println(pod.Status.StartTime)
	var graceperiod int64 = 0

	for i := 0; i < 5; i++ {
		fmt.Println("Attempting to delete the pod. Attempt number: ", i)

		// Update the POD by calling "Update" to the apiserver
		err := c.clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &graceperiod})
		if err != nil {
			fmt.Println(i, "Error deleting pod: ", pod.Name, "\n", err.Error())
			pod, err = c.clientset.CoreV1().Pods("default").Get(ctx, pod.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Println("Error getting POD while deleting: ", err.Error())
			}
		} else {
			fmt.Println("Deleted pod: ", pod.Name, " at time: ", time.Now())
			return true
		}
	}

	return true
}

func (c *controller) handleUpdate(old, cur interface{}) {
	oldp := old.(*corev1.Pod)
	curp := cur.(*corev1.Pod)

	fmt.Println("\n\n Update called for: ", oldp.Name, " current rv: ", curp.ResourceVersion, " old rv: ", oldp.ResourceVersion)

	if curp.ResourceVersion == oldp.ResourceVersion {
		fmt.Println("No change in ResourceVersion for : ", oldp.Name, "\n\n")
		return
	}

	if len(curp.Status.ContainerStatuses) != len(oldp.Status.ContainerStatuses) {
		fmt.Println("@@@ Change in number of containers seen...@@@@@@")
	}

	var found bool
	var killpod bool

	for _, oldstat := range oldp.Status.ContainerStatuses {
		killpod = false
		found = false
		fmt.Println("\n", oldstat.Name, " Old RestartCount: ", oldstat.RestartCount)

		for _, curstat := range curp.Status.ContainerStatuses {
			if curstat.Name == oldstat.Name {
				found = true
				fmt.Println("New RestartCount: ", curstat.RestartCount)

				// If old status was not terminated and new status is terminated, then the container has been killed.
				if oldstat.State.Terminated == nil && curstat.State.Terminated != nil {
					killpod = true
				}
			}
		}

		if found == false {
			fmt.Println("New Status not found for : ", oldstat.Name)
			killpod = true
		}

		if killpod == true {
			// Find the spec of the container which went down
			for _, cont := range oldp.Spec.Containers {
				if cont.Name != oldstat.Name {
					continue
				}
				found = false
				// Find out if the container going down is a critical container.
				// The env variable "pwcriticality" with value of "critical" is a critical container.
				for _, myenv := range cont.Env {
					if myenv.Name == "pwcriticality" && myenv.Value == "critical" {
						fmt.Println("@@@@@@@ Container : ", cont.Name, " is a critical container. Deciding to kill the POD.")
						found = true
						break
					} else {
						killpod = false
					}
				}

				if found == false {
					killpod = false
				}
			}
			break
		}
	}

	if killpod == true {
		fmt.Println("Decided to kill pod ")
		c.killthepod(curp)
	}
}

func (c *controller) worker() {
	//fmt.Println("Called worker at : ", time.Now())
	for c.processItem() {
		// The function processItem will dequeue an item from queue and process it.
		// It will return false if queue is empty or in case of any failure.
		// In that case, this "for" loop will break.
		// Otherwise, this will keep processing items in the queue.
	}
}

func (c *controller) run(ch <-chan struct{}) {
	fmt.Println("starting controller")
	if !cache.WaitForCacheSync(ch, c.podCacheSynced) {
		fmt.Print("waiting for cache to be synced\n")
	}

	// Calls a particular function every 1 sec, until the channel is closed.
	go wait.Until(c.worker, time.Second, ch)

	<-ch
}
