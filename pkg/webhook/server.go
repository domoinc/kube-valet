package webhook

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/op/go-logging"

	"k8s.io/api/admission/v1beta1"

	"github.com/domoinc/kube-valet/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type PodAssigner interface {
	GetPodSchedulingPatches(*corev1.Pod) []utils.JsonPatchOperation
}

type Config struct {
	Listen      string
	TLSCertPath string
	TLSKeyPath  string
}

type Server struct {
	config *Config

	podAssigner PodAssigner

	server *http.Server

	log *logging.Logger
}

func New(c *Config, pa PodAssigner, log *logging.Logger) *Server {
	return &Server{
		config:      c,
		podAssigner: pa,
		log:         log,
	}
}

func (s *Server) Run() {
	pair, err := tls.LoadX509KeyPair(s.config.TLSCertPath, s.config.TLSKeyPath)
	if err != nil {
		s.log.Fatalf("Filed to load key pair: %v", err)
	}

	s.server = &http.Server{
		Addr: s.config.Listen,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{pair},
			CipherSuites: []uint16{
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
			},
		},
	}

	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", s.mutateHandler)
	s.server.Handler = mux

	// Start the server, restart on errors
	for {
		// start webhook server in new rountine
		s.log.Noticef("Starting Webhook Server on \"%s\"", s.config.Listen)
		if err := s.server.ListenAndServeTLS("", ""); err != nil {
			s.log.Errorf("Filed to listen and serve webhook server: %v", err)
		}
		time.Sleep(3 * time.Second)
	}
}

func (s *Server) mutateHandler(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("Processing mutation request")
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		s.log.Error("Empty body in mutation request")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		s.log.Errorf("Invalid Content-Type=%s, expected application/json", contentType)
		http.Error(w, "Invalid Content-Type, expected `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	admissionReview := &v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, admissionReview); err != nil {
		s.log.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// Handle mutations for different resources
	if admissionReview.Request.Kind.Kind == "Pod" {
		s.log.Debug("Processing pod mutation")
		admissionResponse = s.mutatePod(admissionReview, s.podAssigner)
	}

	// Populate admissionReview from admissionResponse
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if admissionReview.Request != nil {
			admissionReview.Response.UID = admissionReview.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		s.log.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}

	if _, err := w.Write(resp); err != nil {
		s.log.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func (s *Server) mutatePod(ar *v1beta1.AdmissionReview, pa PodAssigner) *v1beta1.AdmissionResponse {
	req := ar.Request
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		s.log.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// Inject object metadata from request
	pod.Namespace = req.Namespace

	patchBytes, err := json.Marshal(pa.GetPodSchedulingPatches(&pod))
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	s.log.Debugf("Generated patch: %s\n", patchBytes)
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}
