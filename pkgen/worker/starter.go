package worker

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"time"

	"github.com/panux/builder/pkgen"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

//Starter is a struct which starts workers
type Starter struct {
	kcl       *kubernetes.Clientset //kubernetes Clientset to use for starting worker pods
	namespace string                //kubernetes namespace to start pods in
}

//Start starts a new worker using kubernetes
func (s *Starter) Start(pk *pkgen.PackageGenerator) (w *Worker, err error) {
	//create worker pod struct
	wpod := new(workerPod)
	defer func() {
		if err != nil { //cleanup pod if this failed
			cerr := wpod.Close()
			if cerr != nil && cerr != io.ErrClosedPipe {
				//If this happens then a sysadmin will have to intervene
				//I really cannot think of a better way to handle this now
				log.Printf("Failed to close pod on error: %q\n", cerr.Error())
			}
		}
	}()

	//generate TLS cert
	ctmpl, err := genCertTmpl() //generate cert template
	if err != nil {
		return nil, err
	}
	privkey, err := rsa.GenerateKey(rand.Reader, 4096) //generate RSA key (4096-bit)
	if err != nil {
		return nil, err
	}
	cert, err := x509.CreateCertificate(rand.Reader, ctmpl, ctmpl, privkey.Public(), privkey) //create cert with template and key
	if err != nil {
		return nil, err
	}

	//generate RSA auth key
	authkey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}

	//generate TLS/auth secret
	sec := new(v1.Secret)
	sec.Data = map[string][]byte{
		"srvkey": []byte(base64.StdEncoding.EncodeToString(
			pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(privkey),
			}),
		)),
		"cert": cert,
		"auth": []byte(base64.StdEncoding.EncodeToString(
			pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PUBLIC KEY",
				Bytes: x509.MarshalPKCS1PublicKey(&authkey.PublicKey),
			}),
		)),
	}
	sec.GenerateName = "worker-tls"
	sec, err = s.kcl.CoreV1().Secrets(s.namespace).Create(sec)
	if err != nil {
		return nil, err
	}
	wpod.sslsecret = sec

	//create pod
	pod, err := wpod.genPodSpec(pk)
	if err != nil {
		return
	}
	pod, err = s.kcl.CoreV1().Pods(s.namespace).Create(pod)
	if err != nil {
		return
	}
	wpod.pod = pod

	//TODO: generate tls config and prepare client

	return nil, nil
}

func genCertTmpl() (*x509.Certificate, error) {
	//generate random serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	//certificate not valid before now
	notBefore := time.Now()

	//certificate valid for 12 hours
	notAfter := notBefore.Add(time.Hour * 12)

	//generate cert
	return &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Panux Builder"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}, nil
}
