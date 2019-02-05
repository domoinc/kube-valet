package main // import "github.com/domoinc/kube-valet"

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	valet "github.com/domoinc/kube-valet/pkg/client/clientset/versioned"
	"github.com/op/go-logging"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	resourcelock "k8s.io/client-go/tools/leaderelection/resourcelock"

	valetconfig "github.com/domoinc/kube-valet/pkg/config"
)

const (
	KubernetesComponent          = "kube-valet"
	DefaultElectionConfigmapName = "kube-valet-election"
)

var (
	// DoS vulnerability fix
	// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	netClient = &http.Client{
		Timeout: time.Second * 5,
	}

	// App
	app                = kingpin.New("kube-valet", "Automated QoS for kubernetes")
	logLevel           = app.Flag("loglevel", "Logging level.").Short('L').Default("NOTICE").String()
	inCluster          = app.Flag("in-cluster", "Running In Cluster").Default("false").Bool()
	kubeconfig         = app.Flag("kubeconfig", "Path to kubeconfig").Short('c').String()
	configmapNamespace = app.Flag("configmap-namespace", "Configmap namespace").Default("kube-system").String()
	configmapName      = app.Flag("configmap-name", "Configmap name").Default("kube-valet").String()

	nodeAssignment = app.Flag("node-assignment", "Run the NodeAssignment controllers, Default: true").Default("true").Bool()
	packLeft       = app.Flag("scheduling-packleft", "Run the Pack Left Scheduling controller, Default: true").Default("true").Bool()
	//numNagThreads  = app.Flag("num-nag-threads", "Max number of NodAssignmentGroups that will be reconciled concurrently").Default("1").Int() // Can't enable until node resource locking is in place

	podAssignment = app.Flag("pod-assignment", "Run the PodAssignment Controllers, Default: true").Default("true").Bool()
	numPodThreads = app.Flag("num-pod-threads", "Max number of Pods that will be initilized concurrently").Default("1").Int()

	// Follow naming scheme of upstream elected components like kube-scheduler and kube-controller-manager
	// EX: https://kubernetes.io/docs/reference/generated/kube-scheduler/
	// This makes it easy to copy/paste any election settings to this component
	leaderElection = app.Flag("leader-elect", "Enable Leader Elect").Bool()
	electDuration  = app.Flag("leader-elect-lease-duration", "The duration that non-leaders will wait before attempting to become the leader").Default("30s").Duration()
	electDeadline  = app.Flag("leader-elect-renew-deadline", "The interval between attempts by the acting master to renew leadership before it stops leading. This must be less the lease duration").Default("10s").Duration()
	electResource  = app.Flag("leader-elect-resource-lock", "The type of resource that will be used for the lock. Allowed: configmaps, endpoints").Default(resourcelock.ConfigMapsResourceLock).Enum(resourcelock.ConfigMapsResourceLock, resourcelock.EndpointsResourceLock)
	electRetry     = app.Flag("leader-elect-retry-period", "The duration the clients should wait between attempting acquisition and renewal of a leadership").Default("2s").Duration()
	electName      = app.Flag("lock-object-name", "Name of the election resource to be used for locks").Default(DefaultElectionConfigmapName).String()

	// Allow override of some election properties not configurable in upstream components. Just in case
	electID        = app.Flag("leader-elect-id", "Unique name for election candidate. Defaults to hostname").String()
	electNamespace = app.Flag("leader-elect-namespace", "The namespace that the election resource will be created in").Default("kube-system").String()

	log    = logging.MustGetLogger("kube-valet")
	format = logging.MustStringFormatter(`%{color}%{time:2006-01-02T15:04:05.999Z-07:00} %{shortfunc} : %{level:.4s}%{color:reset} %{message}`)
)

func main() {
	// Enable short help flag
	app.HelpFlag.Short('h')

	// Parse cmd
	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Setup default identity if not specified
	// Default hostname as id
	if *electID == "" {
		hostname, _ := os.Hostname()
		electID = &hostname
	}

	// Setup logging
	logging.SetBackend(logging.NewLogBackend(os.Stdout, "", 0)) // Fix double-timestamp

	logging.SetFormatter(format)

	backend1 := logging.NewLogBackend(os.Stdout, "", 0)

	backend1Leveled := logging.AddModuleLevel(backend1)
	level, err := logging.LogLevel(*logLevel)
	if err != nil {
		fmt.Printf("Invalid log level: %s", *logLevel)
		os.Exit(1)
	}
	backend1Leveled.SetLevel(level, "")

	log.SetBackend(backend1Leveled)

	config := getConfig()

	// create the kubernetes client
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Errorf("Error creating Kubernetes client: %s", err)
		os.Exit(2)
	}

	// create the valet client
	valetClient, err := valet.NewForConfig(config)
	if err != nil {
		log.Errorf("Error creating kube-valet client: %s", err)
		os.Exit(2)
	}

	// Create a new KubeValet
	kd := NewKubeValet(kubeClient, valetClient, &valetconfig.ValetConfig{
		ParController: valetconfig.ControllerConfig{
			Threads:   *numPodThreads,
			ShouldRun: *podAssignment,
		},
		NagController: valetconfig.ControllerConfig{
			Threads:   1,
			ShouldRun: *nodeAssignment,
		},
		PLController: valetconfig.ControllerConfig{
			Threads:   1,
			ShouldRun: *packLeft,
		},
		LoggingBackend: backend1Leveled,
	})

	http.Handle("/metrics", promhttp.Handler())
	go startMetricsHttp()

	// Run the kube valet
	kd.Run()
}

func startMetricsHttp() {
	for true {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Errorf("Metrics server had an error %v", err)
		}
		time.Sleep(20 * time.Second)
	}
}

func getConfig() *rest.Config {
	if *inCluster {
		config, err := rest.InClusterConfig()
		if err != nil {
			log.Errorf("Error getting Kubernetes in-cluster client config: %s", err)
			os.Exit(1)
		}
		return config

	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Errorf("Error getting Kubernetes client config: %s", err)
		os.Exit(1)
	}
	return config
}
