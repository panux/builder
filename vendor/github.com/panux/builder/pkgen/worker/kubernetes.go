package worker

import (
	"context"
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

// workerPod is a struct containing info for manipulating the kubernetes pod.
type workerPod struct {
	//kubernetes Clientset to use when managing the Worker.
	kcl *kubernetes.Clientset

	//pod that worker is in.
	pod *v1.Pod

	//secret that the worket SSL key is in.
	sslsecret *v1.Secret
}

// closePod deletes the pod.
func (wp *workerPod) closePod() error {
	err := wp.kcl.CoreV1().Pods(wp.pod.Namespace).Delete(wp.pod.Name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	wp.pod = nil
	return nil
}

//closeSecret deletes the ssl cert secret.
func (wp *workerPod) closeSecret() error {
	err := wp.kcl.CoreV1().Secrets(wp.sslsecret.Namespace).Delete(wp.sslsecret.Name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	wp.sslsecret = nil
	return nil
}

// ErrSuccess is returned when a pod prematurely exits with a success code.
var ErrSuccess = errors.New("pod status is \"Succeeded\" but the pod should not have terminated yet")

// waitStart waits for pod to start (caller must provide cancellation via context).
func (wp *workerPod) waitStart(ctx context.Context) error {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		//update status of pod
		p, err := wp.kcl.CoreV1().Pods(wp.pod.Namespace).Get(wp.pod.Name, metav1.GetOptions{})
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

		//check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// Close closes the pod.
func (wp *workerPod) Close() error {
	if wp.pod == nil && wp.sslsecret == nil {
		return io.ErrClosedPipe
	}
	if wp.pod != nil {
		err := wp.closePod()
		if err != nil {
			return err
		}
	}
	if wp.sslsecret != nil {
		err := wp.closeSecret()
		if err != nil {
			return err
		}
	}
	return nil
}

// genPodSpec generates a Kubernetes pod spec for the worker.
func (wp *workerPod) genPodSpec(pk *pkgen.PackageGenerator, name string) (*v1.Pod, error) {
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
	case pkgen.BuilderBootstrap:
		img = "panux/worker:alpine"
	case pkgen.BuilderDocker:
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
	case pkgen.BuilderDefault:
		img = "panux/worker"
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name:            "worker",
					Image:           img,
					ImagePullPolicy: v1.PullAlways,
					VolumeMounts:    vmounts,
					ReadinessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(80),
								Path: "/status",
							},
						},
					},
				},
			},
			Volumes: vols,
			NodeSelector: map[string]string{
				"beta.kubernetes.io/arch": pk.HostArch.GoArch(),
			},
		},
	}
	return pod, nil
}
