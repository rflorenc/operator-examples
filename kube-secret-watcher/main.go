package main

import (
	"flag"
	"time"

	"github.com/golang/glog"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rflorenc/kube-secret-watcher/pkg/controller"
	"github.com/rflorenc/kube-secret-watcher/pkg/signals"
)

var (
	kubeConfig string
	masterURL  string
)

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "Path to a kubeConfig. Out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Out-of-cluster.")
}

func main() {
	flag.Parse()

	stopChannel := signals.SignalHandler()

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	if err != nil {
		glog.Fatalf("Error building kubeConfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Error obtaining clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Minute*5)
	watchController := controller.NewController(kubeClient, kubeInformerFactory)

	go kubeInformerFactory.Start(stopChannel)

	if err = watchController.Run(2, stopChannel); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}
