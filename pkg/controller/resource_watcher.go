package controller

import (
	"context"
	"fmt"

	"github.com/op/go-logging"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	assignmentsv1alpha1 "github.com/domoinc/kube-valet/pkg/apis/assignments/v1alpha1"
	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/domoinc/kube-valet/pkg/config"
	"github.com/domoinc/kube-valet/pkg/controller/nodeassignment"
	"github.com/domoinc/kube-valet/pkg/controller/podassignment"
	"github.com/domoinc/kube-valet/pkg/controller/scheduling/packleft"
)

// ResourceWatcher abstracts and shares indexers and informers.
// Passes events onto the controllers that handle the business logic for each event.
type ResourceWatcher struct {
	kubeClient  kubernetes.Interface
	valetClient valet.Interface
	log         *logging.Logger
	config      *config.ValetConfig

	parCtlr *podassignment.Controller
	nagCtlr *nodeassignment.Controller
	plCtlr  *packleft.Controller

	parInformer cache.Controller
	parIndexer  cache.Indexer

	cparInformer cache.Controller
	cparIndexer  cache.Indexer

	nagControllers []NagController
	nagInformer    cache.Controller
	nagIndexer     cache.Indexer

	nodeControllers []NodeController
	nodeInformer    cache.Controller
	nodeIndexer     cache.Indexer

	podControllers []PodController
	podInformer    cache.Controller
	podIndexer     cache.Indexer

	plMan *packleft.Manager
}

// NewResourceWatcher creates a new ResourceWatcher
func NewResourceWatcher(kubeClientet kubernetes.Interface, valetClient valet.Interface, config *config.ValetConfig) *ResourceWatcher {
	return &ResourceWatcher{
		kubeClient:  kubeClientet,
		valetClient: valetClient,
		log:         logging.MustGetLogger("ResourceWatcher"),
		config:      config,
	}
}

func (rw *ResourceWatcher) addNodeController(controller NodeController) {
	rw.nodeControllers = append(rw.nodeControllers, controller)
}

func (rw *ResourceWatcher) addNagController(controller NagController) {
	rw.nagControllers = append(rw.nagControllers, controller)
}

func (rw *ResourceWatcher) addPodController(controller PodController) {
	rw.podControllers = append(rw.podControllers, controller)
}

func (rw *ResourceWatcher) clearAllControllers() {
	// Set all controller slices to nil to clear them out
	// https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices
	rw.nodeControllers = nil
	rw.nagControllers = nil
	rw.podControllers = nil
}

func (rw *ResourceWatcher) StartElectedComponents(ctx context.Context) {
	rw.log.Noticef("Starting elected components")

	if rw.config.ParController.ShouldRun {
		rw.addPodController(rw.parCtlr)
	}

	if rw.config.NagController.ShouldRun {
		rw.addNodeController(rw.nagCtlr)
		rw.addNagController(rw.nagCtlr)
	}

	if rw.config.PLController.ShouldRun {
		rw.addNodeController(rw.plCtlr)
		rw.addNagController(rw.plCtlr)
		rw.addPodController(rw.plCtlr)
	}

	// Force a resync of all watched resources in case something was missed during leader switch
	// All controllers are state-seeking so this is safe to do
	if err := rw.nodeIndexer.Resync(); err != nil {
		rw.log.Errorf("Error during resync after being elected")
	}
}

func (rw *ResourceWatcher) StopElectedComponents() {
	rw.log.Noticef("Stopping elected components")

	// Remove all controllers to stop doing elected tasks
	// Caches will continue to run in order to continue to provide data
	// for webhook requests
	rw.clearAllControllers()
}

func (rw *ResourceWatcher) ParController() *podassignment.Controller {
	return rw.parCtlr
}

// Run starts the indexers, informers, and controllers.
func (rw *ResourceWatcher) Run(stopChan chan struct{}) {
	rw.log.Infof("starting controllers")

	coreRestClient := rw.kubeClient.CoreV1().RESTClient()
	assignmentRestClient := rw.valetClient.AssignmentsV1alpha1().RESTClient()

	//pod controller
	podListWatch := cache.NewListWatchFromClient(coreRestClient, "pods", corev1.NamespaceAll, fields.Everything())

	//TODO: make resync configurable?
	rw.podIndexer, rw.podInformer = cache.NewIndexerInformer(podListWatch, &corev1.Pod{}, 0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*corev1.Pod)
				for _, ctlr := range rw.podControllers {
					ctlr.OnAddPod(pod)
				}
			},
			UpdateFunc: func(oldObj interface{}, newObj interface{}) {
				oldPod := oldObj.(*corev1.Pod)
				newPod := newObj.(*corev1.Pod)
				for _, ctlr := range rw.podControllers {
					ctlr.OnUpdatePod(oldPod, newPod)
				}
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*corev1.Pod)
				for _, ctlr := range rw.podControllers {
					ctlr.OnDeletePod(pod)
				}
			},
		}, cache.Indexers{})

	nodeListWatch := cache.NewListWatchFromClient(coreRestClient, "nodes", corev1.NamespaceAll, fields.Everything())

	//TODO: make resync configurable?
	rw.nodeIndexer, rw.nodeInformer = cache.NewIndexerInformer(nodeListWatch, &corev1.Node{}, 0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				node := obj.(*corev1.Node)
				for _, ctlr := range rw.nodeControllers {
					ctlr.OnAddNode(node)
				}
			},
			UpdateFunc: func(oldObj interface{}, newObj interface{}) {
				oldNode := oldObj.(*corev1.Node)
				newNode := newObj.(*corev1.Node)
				for _, ctlr := range rw.nodeControllers {
					ctlr.OnUpdateNode(oldNode, newNode)
				}
			},
			DeleteFunc: func(obj interface{}) {
				node := obj.(*corev1.Node)
				for _, ctlr := range rw.nodeControllers {
					ctlr.OnDeleteNode(node)
				}
			},
		}, cache.Indexers{})

	nagListWatcher := cache.NewListWatchFromClient(assignmentRestClient, "nodeassignmentgroups", corev1.NamespaceAll, fields.Everything())

	//TODO: make resync configurable?
	rw.nagIndexer, rw.nagInformer = cache.NewIndexerInformer(nagListWatcher, &assignmentsv1alpha1.NodeAssignmentGroup{}, 0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
				for _, ctlr := range rw.nagControllers {
					ctlr.OnAddNag(nag)
				}
			},
			UpdateFunc: func(oldObj interface{}, newObj interface{}) {
				oldNag := oldObj.(*assignmentsv1alpha1.NodeAssignmentGroup)
				newNag := newObj.(*assignmentsv1alpha1.NodeAssignmentGroup)
				for _, ctlr := range rw.nagControllers {
					ctlr.OnUpdateNag(oldNag, newNag)
				}
			},
			DeleteFunc: func(obj interface{}) {
				nag := obj.(*assignmentsv1alpha1.NodeAssignmentGroup)
				for _, ctlr := range rw.nagControllers {
					ctlr.OnDeleteNag(nag)
				}
			},
		}, cache.Indexers{})

	parListWatcher := cache.NewListWatchFromClient(assignmentRestClient, "podassignmentrules", corev1.NamespaceAll, fields.Everything())
	//TODO: make resync configurable?
	rw.parIndexer, rw.parInformer = cache.NewIndexerInformer(parListWatcher, &assignmentsv1alpha1.PodAssignmentRule{}, 0, cache.ResourceEventHandlerFuncs{}, cache.Indexers{})

	cparListWatcher := cache.NewListWatchFromClient(assignmentRestClient, "clusterpodassignmentrules", corev1.NamespaceAll, fields.Everything())
	//TODO: make resync configurable?
	rw.cparIndexer, rw.cparInformer = cache.NewIndexerInformer(cparListWatcher, &assignmentsv1alpha1.ClusterPodAssignmentRule{}, 0, cache.ResourceEventHandlerFuncs{}, cache.Indexers{})

	// Initialize controllers
	rw.parCtlr = podassignment.NewController(rw.podIndexer, rw.cparIndexer, rw.parIndexer, rw.kubeClient, rw.valetClient, rw.config.ParController.Threads, stopChan)
	rw.nagCtlr = nodeassignment.NewController(rw.nagIndexer, rw.nodeIndexer, rw.kubeClient, rw.valetClient, rw.config.NagController.Threads, stopChan)
	rw.plCtlr = packleft.NewController(rw.nagIndexer, rw.nodeIndexer, rw.podIndexer, rw.kubeClient, rw.valetClient, rw.config.PLController.Threads, stopChan)

	// start caches
	go rw.podInformer.Run(stopChan)
	rw.log.Infof("starting node informer")
	go rw.nodeInformer.Run(stopChan)
	rw.log.Infof("starting nag informer")
	go rw.nagInformer.Run(stopChan)
	rw.log.Infof("starting par informer")
	go rw.parInformer.Run(stopChan)
	rw.log.Infof("starting cpar informer")
	go rw.cparInformer.Run(stopChan)

	rw.waitForCacheSync(stopChan, rw.podInformer, "pod")
	rw.waitForCacheSync(stopChan, rw.nodeInformer, "node")
	rw.waitForCacheSync(stopChan, rw.nagInformer, "nag")
	rw.waitForCacheSync(stopChan, rw.parInformer, "par")
	rw.waitForCacheSync(stopChan, rw.cparInformer, "cpar")

	// start controller queue processing
	if rw.config.NagController.ShouldRun {
		rw.log.Info("starting nag controller")
		go rw.nagCtlr.Run()
	}
	if rw.config.PLController.ShouldRun {
		rw.log.Info("starting pack left controller")
		go rw.plCtlr.Run()
	}
}

func (rw *ResourceWatcher) waitForCacheSync(stopChan chan struct{}, informer cache.Controller, infType string) {
	if !cache.WaitForCacheSync(stopChan, informer.HasSynced) {
		msg := fmt.Errorf("timed out waiting for %s cache to sync", infType)
		runtime.HandleError(msg)
		panic(msg) //TODO: is this an error worthy of rebooting?
	}
	rw.log.Infof("%s cache has synced", infType)
}
