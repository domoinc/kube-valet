package main

import (
	"context"

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
	"github.com/domoinc/kube-valet/pkg/webhook"
)

type KubeValet struct {
	kubeClient  kubernetes.Interface
	valetClient valet.Interface
	stopChan    chan struct{}
	config      *config.ValetConfig
}

func NewKubeValet(kc kubernetes.Interface, dc valet.Interface, config *config.ValetConfig) *KubeValet {
	logging.SetBackend(config.LoggingBackend)
	return &KubeValet{
		kubeClient:  kc,
		valetClient: dc,
		config:      config,
	}
}

func (kd *KubeValet) Run() {
	log.Notice("Running kube-valet controller")
	ctx := context.Background()

	// Setup and start resource watcher
	resourceWatcher := controller.NewResourceWatcher(kd.kubeClient, kd.valetClient, kd.config)
	kd.stopChan = make(chan struct{})
	resourceWatcher.Run(kd.stopChan)

	// Start the webhook server
	whConfig := &webhook.Config{
		Listen:      *listen,
		TLSCertPath: *tlsCertPath,
		TLSKeyPath:  *tlsKeyPath,
	}
	mwhs := webhook.New(
		whConfig,
		resourceWatcher.ParController().PodManager(),
		log,
	)
	go mwhs.Run()

	// Handle elected processes
	if *leaderElection {
		log.Debug("Leader election enabled")

		log.Debug("Building ResourceLock")
		rl, err := resourcelock.New(
			*electResource,
			*electNamespace,
			*electName,
			kd.kubeClient.CoreV1(),
			kd.kubeClient.CoordinationV1(),
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

		leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
			Lock:          rl,
			LeaseDuration: *electDuration,
			RenewDeadline: *electDeadline,
			RetryPeriod:   *electRetry,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: resourceWatcher.StartElectedComponents,
				OnStoppedLeading: resourceWatcher.StopElectedComponents,
				OnNewLeader: func(identity string) {
					log.Debugf("Observed %s as the leader", identity)
				},
			},
		})
	} else {
		log.Notice("Leader election disabled")
		resourceWatcher.StartElectedComponents(ctx)
		<-ctx.Done()
	}
}
