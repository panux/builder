package worker

import (
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/panux/builder/pkgen"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

//workerPod is a struct containing info for manipulating the kubernetes pod
type workerPod struct {
	kcl       *kubernetes.Clientset //kubernetes Clientset to use when managing the Worker
	pod       *v1.Pod               //pod that worker is in
	sslsecret *v1.Secret            //secret that the worket SSL key is in
}

//delete pod
func (wp *workerPod) closePod() error {
	err := wp.kcl.CoreV1().Pods(wp.pod.Namespace).Delete(wp.pod.Name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	wp.pod = nil
	return nil
}

//delete ssl cert secret
func (wp *workerPod) closeSecret() error {
	err := wp.kcl.CoreV1().Secrets(wp.sslsecret.Namespace).Delete(wp.sslsecret.Name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	wp.sslsecret = nil
	return nil
}

//ErrSuccess is returned when a pod prematurely exits with a success code
var ErrSuccess = errors.New("pod status is \"Succeeded\" but the pod should not have terminated yet")

//TimeoutErr is an error returned on timeout
type TimeoutErr struct {
	Waited time.Duration //Waited is the time waited before returning error
}

func (te TimeoutErr) Error() string {
	return fmt.Sprintf("timed out after %s", te.Waited.String())
}

func (te TimeoutErr) String() string {
	return te.Error()
}

//wait for pod to start (with 10 min timeout)
func (wp *workerPod) waitStart() error {
	//get start time for timeout
	start := time.Now()

	for {
		//update status of pod
		p, err := wp.kcl.CoreV1().Pods(wp.pod.Namespace).UpdateStatus(wp.pod)
		if err != nil {
			return err
		}
		wp.pod = p

		//check pod phase
		switch wp.pod.Status.Phase {
		case v1.PodPending:
			//still pending
		case v1.PodRunning:
			return nil //its up!
		case v1.PodSucceeded:
			return ErrSuccess
		case v1.PodFailed:
			return fmt.Errorf("pod failed: %q", wp.pod.Status.Message)
		default:
			//log it and hope that it goes away eventually
			log.Printf("Unrecognized kubernetes pod phase: %q\n", string(wp.pod.Status.Phase))
		}

		//check for timeout (10 min)
		waited := time.Since(start)
		if waited > time.Minute*10 {
			return TimeoutErr{waited}
		}

		//wait 5 seconds before retrying
		time.Sleep(5 * time.Second)
	}
}

//close pod
func (wp *workerPod) Close() error {
	if wp.pod == nil && wp.sslsecret == nil {
		return io.ErrClosedPipe
	}
	if wp.pod == nil {
		err := wp.closePod()
		if err != nil {
			return err
		}
	}
	if wp.sslsecret == nil {
		err := wp.closeSecret()
		if err != nil {
			return err
		}
	}
	return nil
}

func (wp *workerPod) genPodSpec(pk *pkgen.PackageGenerator) (*v1.Pod, error) {
	var img string
	vols := []v1.Volume{
		v1.Volume{
			Name: "srvkey",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: wp.sslsecret.Name,
					Items: []v1.KeyToPath{
						v1.KeyToPath{
							Key:  "srvkey",
							Path: "srvkey.pem",
						},
						v1.KeyToPath{
							Key:  "cert",
							Path: "cert.pem",
						},
						v1.KeyToPath{
							Key:  "auth",
							Path: "auth.pem",
						},
					},
				},
			},
		},
	}
	vmounts := []v1.VolumeMount{
		v1.VolumeMount{
			Name:      "srvkey",
			ReadOnly:  true,
			MountPath: "/srv/secret/",
		},
	}
	switch pk.Builder {
	case "bootstrap":
		img = "panux/worker:alpine"
	case "docker":
		hpt := v1.HostPathSocket
		vols = append(vols, v1.Volume{
			Name: "dockersock",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/var/run/docker.sock",
					Type: &hpt,
				},
			},
		})
		vmounts = append(vmounts, v1.VolumeMount{
			Name:      "dockersock",
			ReadOnly:  false,
			MountPath: "/var/run/docker.sock",
		})
		fallthrough
	case "default":
		img = "panux/worker"
	default:
		return nil, fmt.Errorf("unrecognized builder: %q", pk.Builder)
	}
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name:            "worker",
					Image:           img,
					ImagePullPolicy: v1.PullAlways,
					VolumeMounts:    vmounts,
					ReadinessProbe: &v1.Probe{
						Handler: v1.Handler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.FromInt(20),
							},
						},
					},
				},
			},
			Volumes: vols,
		},
	}
	return pod, nil
}
