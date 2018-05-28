package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	var secretName string
	var namespace string
	var keyBits int
	flag.StringVar(&secretName, "secret", "panuxAuth", "kubernetes secret to create")
	flag.StringVar(&namespace, "namespace", "default", "kubernetes namespace to use")
	flag.IntVar(&keyBits, "bits", 8192, "RSA key length")
	flag.Parse()

	// prep RSA key
	log.Println("Generating RSA key. . .")
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		log.Fatalf("Failed to generate RSA key: %q\n", err.Error())
	}
	log.Println("Validating RSA key. . .")
	err = key.Validate()
	if err != nil {
		log.Fatalf("Failed to validate RSA key: %q\n", err.Error())
	}

	// encode key
	log.Println("Encoding key. . .")
	privdat := x509.MarshalPKCS1PrivateKey(key)
	pubdat := x509.MarshalPKCS1PublicKey(&key.PublicKey)

	// create a temporary directory to work in
	log.Println("Prepping temporary directory. . .")
	dir, err := ioutil.TempDir("", "pkube")
	if err != nil {
		log.Fatalf("Failed to create temporary dir: %q\n", err.Error())
	}
	defer os.RemoveAll(dir)

	// write authkeys.json
	log.Println("Creating authkeys.json. . .")
	af, err := os.OpenFile(filepath.Join(dir, "authkeys.json"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to create authkeys.json: %q\n", err.Error())
	}
	err = json.NewEncoder(af).Encode([][]byte{pubdat})
	cerr := af.Close()
	if err != nil {
		log.Fatalf("Failed to save authkeys.json: %q\n", err.Error())
	}
	err = cerr
	if err != nil {
		log.Fatalf("Failed to close authkeys.json: %q\n", err.Error())
	}

	// create manager.pem
	log.Println("Creating manager.pem. . .")
	mf, err := os.OpenFile(filepath.Join(dir, "manager.pem"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to create manager.pem: %q\n", err.Error())
	}
	_, err = mf.Write(privdat)
	cerr = mf.Close()
	if err != nil {
		log.Fatalf("Failed to save manager.pem: %q\n", err.Error())
	}
	err = cerr
	if err != nil {
		log.Fatalf("Failed to close manager.pem: %q\n", err.Error())
	}

	// create secret
	log.Println("Creating secret. . .")
	cmd := exec.Command(
		"kubectl", "--namespace", namespace,
		"create", "generic", secretName,
		"--from-file="+filepath.Join(dir, "authkeys.json"),
		"--from-file="+filepath.Join(dir, "manager.pem"),
	)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Kubernetes failed: %q\n", err.Error())
	}

	// done
	log.Println("Done!")
}
