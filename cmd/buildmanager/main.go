package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	//setup kubernetes client
	log.Println("Running kubernetes setup. . . ")
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to setup in cluster config: %q\n", err.Error())
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create client with config: %q\n", err.Error())
	}
	//generate ssh login key
	log.Println("Generating SSH login key. . . ")
	var loginkey ssh.Signer
	var authorizedkeys []byte
	{
		//generate key
		privkey, err := rsa.GenerateKey(rand.Reader, 8192)
		if err != nil {
			log.Fatalf("Failed to generate RSA key: %q\n", err.Error())
		}
		err = privkey.Validate()
		if err != nil {
			log.Fatalf("Failed to validate RSA key: %q\n", err.Error())
		}
		//create SSH signer
		signer, err := ssh.NewSignerFromKey(privkey)
		if err != nil {
			log.Fatalf("Failed to create SSH signer: %q\n", err.Error())
		}
		//create public key
		pubkey, err := ssh.NewPublicKey(&privkey.PublicKey)
		if err != nil {
			log.Fatalf("Failed to create SSH public key: %q\n", err.Error())
		}
		//marshal authorized_keys
		authk := ssh.MarshalAuthorizedKey(pubkey)
		//save output
		loginkey, authorizedkeys = signer, authk
	}
	_ = loginkey
	_ = authorizedkeys
	log.Println("Running HTTP setup. . . ")
	//setup build handler
	http.Handle("/build", websocket.Handler(func(c *websocket.Conn) {
	}))
	//handle build requests
	http.HandleFunc("/build", func(w http.ResponseWriter, r *http.Request) {
		//generate ssh server key
		skey, err := rsa.GenerateKey(rand.Reader, 4096)
	skeyerr:
		if err != nil {
			http.Error(w, "setup error", http.StatusInternalServerError)
			log.Printf("Failed to generate SSH server key: %q\n", err.Error())
			return
		}
		err = skey.Validate()
		if err != nil {
			goto skeyerr
		}
		srvkeydat := pem.EncodeToMemory(&pem.Block{
			Type:    "RSA PRIVATE KEY",
			Headers: nil,
			Bytes:   x509.MarshalPKCS1PrivateKey(skey),
		})
		//store key into secret
		sec := new(v1.Secret)
		sec.Data = map[string][]byte{
			"srvkey": []byte(base64.StdEncoding.EncodeToString(srvkeydat)),
		}
		sec.GenerateName = "srvkey"
		sec, err = client.CoreV1().Secrets("default").Create(sec)
		if err != nil {
			http.Error(w, "setup error", http.StatusInternalServerError)
			log.Printf("Failed to create secret: %q\n", err.Error())
			return
		}
		defer func() {
			delerr := client.CoreV1().Secrets("default").
				Delete(sec.Name, &metav1.DeleteOptions{})
			if delerr != nil {
				log.Printf("Failed to delete secret %q: %q\n", sec.Name, err.Error())
			}
		}()
		//start worker pod
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					v1.Container{
						Name:            "worker",
						Image:           "panux/worker",
						ImagePullPolicy: v1.PullAlways,
						VolumeMounts: []v1.VolumeMount{
							v1.VolumeMount{
								Name:      "srvkey",
								ReadOnly:  true,
								MountPath: "/srv/srvkey.pem",
								SubPath:   "srvkey.pem",
							},
						},
						ReadinessProbe: &v1.Probe{
							Handler: v1.Handler{
								TCPSocket: &v1.TCPSocketAction{
									Port: intstr.FromInt(20),
								},
							},
						},
					},
				},
				Volumes: []v1.Volume{
					v1.Volume{
						Name: "srvkey",
						VolumeSource: v1.VolumeSource{
							Secret: &v1.SecretVolumeSource{
								SecretName: sec.Name,
								Items: []v1.KeyToPath{
									v1.KeyToPath{
										Key:  "srvkey",
										Path: "srvkey.pem",
									},
								},
							},
						},
					},
				},
			},
		}
		pod, err = client.CoreV1().Pods("default").Create(pod)
		if err != nil {
			http.Error(w, "setup error", http.StatusInternalServerError)
			log.Printf("Failed to create pod: %q\n", err.Error())
			return
		}
		defer func() {
			prop := metav1.DeletePropagationBackground
			delerr := client.CoreV1().Pods("default").Delete(pod.Name, &metav1.DeleteOptions{
				PropagationPolicy: &prop,
			})
			if delerr != nil {
				log.Printf("Failed to delete pod %q: %q\n", sec.Name, err.Error())
			}
		}()
		//wait for pod to go up
		for pod.Status.Phase != v1.PodRunning {
			time.Sleep(time.Second)
			pod, err = client.CoreV1().Pods("default").Get(pod.Name, metav1.GetOptions{
				IncludeUninitialized: true,
				//might need to modify ResourceVersion to deal with cache
			})
			if err != nil {
				http.Error(w, "setup error", http.StatusInternalServerError)
				log.Printf("Pod get request failed: %q\n", err.Error())
				return
			}
		}
	})
}
