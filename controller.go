package main

import (
	"github.com/op/go-logging"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"

	"github.com/domoinc/kube-valet/pkg/config"
	"github.com/domoinc/kube-valet/pkg/controller"
)

type KubeValet struct {
	kubeClient  kubernetes.Interface
	valetClient valet.Interface
	stopChan       chan struct{}
	config         *config.ValetConfig
}

func NewKubeValet(kc kubernetes.Interface, dc valet.Interface, config *config.ValetConfig) *KubeValet {
	logging.SetBackend(config.LoggingBackend)
	return &KubeValet{
		kubeClient:  kc,
		valetClient: dc,
		config:         config,
	}
}

func (kd *KubeValet) StartControllers(stop <-chan struct{}) {
	resourceWatcher := controller.NewResourceWatcher(kd.kubeClient, kd.valetClient, kd.config)
	resourceWatcher.Run(kd.stopChan)
}

func (kd *KubeValet) StopControllers() {
	close(kd.stopChan)
}

func (kd *KubeValet) Run() {
	// Create a channel for leader elect events
	// and exit signaling
	kd.stopChan = make(chan struct{})
	defer close(kd.stopChan)

	log.Notice("Running controllers")
	// Do Election
	if *leaderElection {
		log.Debug("Leader election enabled")

		log.Debug("Building ResourceLock")
		rl, err := resourcelock.New(
			*electResource,
			*electNamespace,
			*electName,
			kd.kubeClient.CoreV1(),
			resourcelock.ResourceLockConfig{
				Identity: *electID,
				EventRecorder: record.NewBroadcaster().NewRecorder(
					scheme.Scheme,
					corev1.EventSource{Component: KubernetesComponent},
				),
			},
		)
		if err != nil {
			log.Fatalf("Error building ResourceLock: %s", err)
		}

		log.Debug("Building LeaderElector")

		leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
			Lock:          rl,
			LeaseDuration: *electDuration,
			RenewDeadline: *electDeadline,
			RetryPeriod:   *electRetry,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: kd.StartControllers,
				OnStoppedLeading: kd.StopControllers,
				OnNewLeader: func(identity string) {
					log.Debugf("Observed %s as the leader", identity)
				},
			},
		})
	} else {
		log.Debug("Leader election disabled. Running controllers.")
		kd.StartControllers(kd.stopChan)
		<-kd.stopChan
	}
}
