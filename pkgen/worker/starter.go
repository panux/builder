package worker

import (
	"k8s.io/client-go/kubernetes"
)

//Starter is a struct which starts workers
type Starter struct {
	kcl       *kubernetes.Clientset //kubernetes Clientset to use for starting worker pods
	namespace string                //kubernetes namespace to start pods in
}
