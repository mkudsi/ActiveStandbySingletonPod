package main

import (
	"flag"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "/home/ubuntu/.kube/config", "location to your kubeconfig file")
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		// handle error
		fmt.Printf("erorr %s building config from flags\n", err.Error())
		config, err = rest.InClusterConfig()
		if err != nil {
			fmt.Printf("error %s, getting inclusterconfig", err.Error())
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		// handle error
		fmt.Printf("error %s, creating clientset\n", err.Error())
	}

	//	options := metav1.ListOptions{
	//		LabelSelector: "app=<APPNAME>",
	//	}

	ch := make(chan struct{})
	//informers := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	informers := informers.NewSharedInformerFactoryWithOptions(clientset, 10*time.Minute,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = "app=hapod"
		}))

	c := newController(clientset, informers.Core().V1().Pods())
	informers.Start(ch)
	c.run(ch)
	fmt.Println(informers)
}
