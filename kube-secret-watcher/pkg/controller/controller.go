package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	v1 "k8s.io/client-go/listers/core/v1"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	kubeinformers "k8s.io/client-go/informers"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	kswcst "github.com/rflorenc/kube-secret-watcher/constants"
	kswutil "github.com/rflorenc/kube-secret-watcher/pkg/util"
)

var (
	// contains last SHA256 hash of the secret
	lastHashAnnotation = func(kind, name string) string {
		return fmt.Sprintf("watcher.k8s.io/%s-%s-last-hash", kind, name)
	}
)

// WatcherController is the controller implementation for Secret resources
type WatcherController struct {
	client            kubernetes.Interface
	secretsLister     v1.SecretLister
	secretsSynced     cache.InformerSynced
	deploymentClient  appsv1client.DeploymentsGetter
	deploymentsSynced cache.InformerSynced
	deploymentsIndex  cache.Indexer
	workqueue         workqueue.RateLimitingInterface
	recorder          record.EventRecorder
	getSHA1hashFn     func(interface{}) string
}

// NewController returns a new secret watcher controller
func NewController(
	kubeclientset kubernetes.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory) *WatcherController {

	secretInformer := kubeInformerFactory.Core().V1().Secrets()
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})

	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: kswcst.ControllerEventSourceComponent})

	controller := &WatcherController{
		client:           kubeclientset,
		deploymentClient: kubeclientset.AppsV1(),
		workqueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "data-version"),
		recorder:         recorder,
		secretsLister:    secretInformer.Lister(),
		getSHA1hashFn:    kswutil.GetSHA1hash,
	}

	controller.secretsSynced = secretInformer.Informer().HasSynced
	controller.deploymentsSynced = deploymentInformer.Informer().HasSynced

	deploymentInformer.Informer().AddIndexers(cache.Indexers{
		"secret": indexDeploymentsWatchingSecrets,
	})
	controller.deploymentsIndex = deploymentInformer.Informer().GetIndexer()

	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			controller.enqueueSecret(nil, new)
		},
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueSecret(old, new)
		},
	})

	return controller
}

func indexDeploymentsWatchingSecrets(obj interface{}) ([]string, error) {
	var configs []string
	deployment, ok := obj.(*appsv1.Deployment)
	if !ok {
		return configs, fmt.Errorf("object is not deployment")
	}
	annotations := deployment.GetAnnotations()
	if len(annotations) == 0 {
		return configs, nil
	}
	if triggers, ok := annotations[kswcst.WatcherSecretsAnnotation]; ok {
		secrets := sets.NewString(strings.Split(triggers, ",")...)
		for _, v := range deployment.Spec.Template.Spec.Volumes {
			if v.Secret != nil {
				if secrets.Has(v.Secret.SecretName) {
					configs = append(configs, v.Secret.SecretName)
				}
			}
		}
	}
	return configs, nil
}

// Run starts the controller
func (c *WatcherController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting trigger controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync ...")
	if ok := cache.WaitForCacheSync(stopCh, c.secretsSynced, c.deploymentsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers ...")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers ...")
	return nil
}

func (c *WatcherController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *WatcherController) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if syncErr := c.syncHandler(key); syncErr != nil {
			return fmt.Errorf("error syncing '%s': %s", key, syncErr.Error())
		}
		c.workqueue.Forget(obj)
		return nil
	}(obj)
	if err != nil {
		utilruntime.HandleError(err)
	}
	return true
}

func (c *WatcherController) enqueueObject(prefix string, old, new interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(new)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(prefix + key)
}

func (c *WatcherController) enqueueSecret(old, new interface{}) {
	c.enqueueObject(kswcst.SecretPrefix, old, new)
}

func (c *WatcherController) syncHandler(key string) error {
	parts := strings.Split(key, "#")
	if len(parts) != 2 {
		utilruntime.HandleError(fmt.Errorf("unexpected resource key: %s", key))
		return nil
	}
	kind := parts[0]

	namespace, name, err := cache.SplitMetaNamespaceKey(parts[1])
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", parts[1]))
		return nil
	}

	var (
		obj interface{}
	)

	switch kind {
	case "secret":
		obj, err = c.secretsLister.Secrets(namespace).Get(name)
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
	default:
		utilruntime.HandleError(fmt.Errorf("Only Secret resources are allowed, got: %s", kind))
		return nil
	}

	// First update the data hashes into Secret annotations.
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		utilruntime.HandleError(err)
	}

	// Get all deployments that use the Secret
	toTrigger, err := c.deploymentsIndex.ByIndex(kind, objMeta.GetName())
	if err != nil {
		return err
	}

	// No watchers active for this secret
	if len(toTrigger) == 0 {
		glog.V(5).Infof("%s %q is not watching any deployment", kind, objMeta.GetName())
		return nil
	}

	newDataHash := c.getSHA1hashFn(obj)
	if len(newDataHash) == 0 {
		return nil
	}
	oldDataHash := objMeta.GetAnnotations()[kswcst.HashAnnotation]

	if newDataHash != oldDataHash {
		switch kind {
		case "secret":
			objCopy := obj.(*corev1.Secret).DeepCopy()
			if objCopy.Annotations == nil {
				objCopy.Annotations = map[string]string{}
			}
			objCopy.Annotations[kswcst.HashAnnotation] = newDataHash
			glog.V(3).Infof("Updating secret %s/%s with new data hash: %v", objCopy.Namespace, objCopy.Name, newDataHash)
			if _, err := c.client.CoreV1().Secrets(objCopy.Namespace).Update(objCopy); err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}
		}
	} else {
		glog.V(5).Infof("No change detected in hash for %s %s/%s", kind, objMeta.GetNamespace(), objMeta.GetName())
	}

	// Determine whether to trigger these deployments
	var triggerErrors []error
	for _, obj := range toTrigger {
		dMeta, err := meta.Accessor(obj)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to get accessor for %#v", err))
			continue
		}
		deployment, err := c.client.AppsV1().Deployments(dMeta.GetNamespace()).Get(dMeta.GetName(), meta_v1.GetOptions{})
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to get deployment %s/%s: %v", dMeta.GetNamespace(), dMeta.GetName(), err))
		}
		glog.V(3).Infof("Processing deployment %s/%s that tracks %s %s ...", deployment.Namespace, deployment.Name, kind, objMeta.GetName())

		annotations := deployment.Spec.Template.Annotations
		if annotations == nil {
			annotations = map[string]string{}
		}
		triggerAnnotationKey := lastHashAnnotation(kind, objMeta.GetName())
		if hash, exists := annotations[triggerAnnotationKey]; exists && hash == newDataHash {
			glog.V(3).Infof("Deployment %s/%s already have latest %s %q", deployment.Namespace, deployment.Name, kind, objMeta.GetName())
			continue
		}

		glog.V(3).Infof("Deployment %s/%s has old %s %q and will rollout", deployment.Namespace, deployment.Name, kind, objMeta.GetName())
		annotations[triggerAnnotationKey] = newDataHash

		newDeployment := deployment.DeepCopy()
		newDeployment.Spec.Template.Annotations = annotations
		if _, err := c.client.AppsV1().Deployments(deployment.Namespace).Update(newDeployment); err != nil {
			glog.Errorf("Failed to update deployment %s/%s: %v", deployment.Namespace, deployment.Name, err)
			triggerErrors = append(triggerErrors, err)
		}
	}

	if len(triggerErrors) != 0 {
		return errorsutil.NewAggregate(triggerErrors)
	}

	return nil
}
