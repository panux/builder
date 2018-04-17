package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/panux/builder/bmapi"
	"github.com/panux/builder/pkgen"
	"github.com/panux/builder/pkgen/dlapi"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	var dlserver string
	flag.StringVar(&dlserver, "dlserver", "", "Download server to use (direct if empty)")
	dload := pkgen.NewHTTPLoader(http.DefaultClient, 100*1024*1024)
	if dlserver != "" {
		u, err := url.Parse(dlserver)
		if err != nil {
			log.Fatalf("Failed to download: %q\n", err.Error())
		}
		dload = dlapi.NewDlClient(u, http.DefaultClient)
	}
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
		bmapi.WorkMsgConn(c, func(w *bmapi.MsgStreamWriter, r *bmapi.MsgStreamReader) error {
			errch := make(chan error)
			defer close(errch)
			//handle reading messages
			rch := make(chan bmapi.Message)
			defer close(rch)
			droperr := func() {
				err := recover()
				if err != nil {
					log.Printf("Dropped error: %v\n", err)
				}
			}
			go func() {
				defer droperr()
				for {
					m, err := r.NextMessage()
					if err != nil {
						errch <- err
						return
					}
					rch <- m
				}
			}()
			err := w.Send(bmapi.LogMessage{
				Text:   "Generating SSH keys",
				Stream: 3,
			})
			if err != nil {
				return err
			}
			//generate ssh server key
			skey, err := rsa.GenerateKey(rand.Reader, 4096)
			if err != nil {
				w.Send(
					bmapi.ErrorMessage(
						fmt.Sprintf(
							"Failed to generate SSH server key: %q\n",
							err.Error(),
						),
					),
				)
				return err
			}
			err = skey.Validate()
			if err != nil {
				w.Send(
					bmapi.ErrorMessage(
						fmt.Sprintf(
							"Failed to generate SSH server key: %q\n",
							err.Error(),
						),
					),
				)
				return err
			}
			srvpub, err := ssh.NewPublicKey(skey.PublicKey)
			if err != nil {
				return err
			}
			srvkeydat := pem.EncodeToMemory(&pem.Block{
				Type:    "RSA PRIVATE KEY",
				Headers: nil,
				Bytes:   x509.MarshalPKCS1PrivateKey(skey),
			})
			err = w.Send(bmapi.LogMessage{
				Text:   "Done generating SSH keys",
				Stream: 3,
			})
			if err != nil {
				return err
			}
			//wait for pkgen
			err = w.Send(bmapi.LogMessage{
				Text:   "Waiting for pkgen. . . ",
				Stream: 3,
			})
			if err != nil {
				return err
			}
			var pk *pkgen.PackageGenerator
			{
				select {
				case m := <-rch:
					pkm, ok := m.(bmapi.PkgenMessage)
					if !ok {
						perr := bmapi.ErrorMessage("Protocol error: first message is not a PkgenMessage")
						w.Send(perr)
						return perr
					}
					pk = pkm.Gen
				case err = <-errch:
					w.Send(bmapi.ErrorMessage(err.Error()))
					return err
				}
			}
			_ = srvkeydat
			_ = pk
			//TODO: kubernetes
			//store key into secret
			sec := new(v1.Secret)
			sec.Data = map[string][]byte{
				"srvkey": []byte(base64.StdEncoding.EncodeToString(srvkeydat)),
			}
			sec.GenerateName = "srvkey"
			sec, err = client.CoreV1().Secrets("default").Create(sec)
			if err != nil {
				w.Send(bmapi.ErrorMessage(fmt.Sprintf("Failed to create secret: %q\n", err.Error())))
				return err
			}
			defer func() {
				delerr := client.CoreV1().Secrets("default").
					Delete(sec.Name, &metav1.DeleteOptions{})
				if delerr != nil {
					log.Printf("Failed to delete secret %q: %q\n", sec.Name, err.Error())
				}
			}()
			//start worker pod
			var img string
			vols := []v1.Volume{
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
			}
			vmounts := []v1.VolumeMount{
				v1.VolumeMount{
					Name:      "srvkey",
					ReadOnly:  true,
					MountPath: "/srv/srvkey.pem",
					SubPath:   "srvkey.pem",
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
				pe := bmapi.ErrorMessage(fmt.Sprintf("unrecognized builder: %q\n", pk.Builder))
				w.Send(pe)
				return pe
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
			pod, err = client.CoreV1().Pods("default").Create(pod)
			if err != nil {
				w.Send(bmapi.ErrorMessage(fmt.Sprintf("Failed to create pod: %q\n", err.Error())))
				return err
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
			err = w.Send(bmapi.LogMessage{
				Text:   "Waiting for pod to start up. . . ",
				Stream: 3,
			})
			if err != nil {
				return err
			}
			twait := time.Now()
			//wait for pod to go up
			for pod.Status.Phase != v1.PodRunning {
				switch pod.Status.Phase { //handle bad phases
				case v1.PodFailed:
					we := bmapi.ErrorMessage("pod failed")
					w.Send(we)
					return we
				case v1.PodSucceeded:
					we := bmapi.ErrorMessage("pod reported as succeeded, but should not have terminated")
					w.Send(we)
					return we
				case v1.PodUnknown:
					log.Printf("State of pod %q unknown. Retrying soon.\n", pod.Name)
				case v1.PodPending: //it is pending - keep going
				default:
					we := bmapi.ErrorMessage(fmt.Sprintf("pod in unrecognized phase %q", string(pod.Status.Phase)))
					w.Send(we)
					return we
				}
				if time.Since(twait) > 5*time.Minute { //give up after 5 min
					we := bmapi.ErrorMessage("timeout on pod startup")
					w.Send(we)
					return we
				}
				time.Sleep(time.Second)
				pod, err = client.CoreV1().Pods("default").Get(pod.Name, metav1.GetOptions{
					IncludeUninitialized: true,
					//might need to modify ResourceVersion to deal with cache
				})
				if err != nil {
					w.Send(bmapi.ErrorMessage(fmt.Sprintf("Pod get request failed: %q\n", err.Error())))
					return err
				}
			}
			//connect to pod over SSH
			err = w.Send(bmapi.LogMessage{
				Text:   "connecting to pod over SSH. . . ",
				Stream: 3,
			})
			if err != nil {
				return err
			}
			cli, err := ssh.Dial("tcp", pod.Status.PodIP, &ssh.ClientConfig{
				User:            "root",
				Auth:            []ssh.AuthMethod{ssh.PublicKeys(loginkey)},
				HostKeyCallback: ssh.FixedHostKey(srvpub),
				Timeout:         time.Minute,
			})
			if err != nil {
				w.Send(bmapi.ErrorMessage(fmt.Sprintf("failed to connect SSH: %q\n", err.Error())))
				return err
			}
			defer cli.Close()
			sb := &sshBot{
				cli: cli,
			}
			//do stuff
			logch := make(chan bmapi.LogMessage)
			pkrch := make(chan bmapi.PackageRequestMessage)
			defer close(pkrch)
			go func() {
				defer close(pkrch)
				//create build dir
				err := sb.RunCmd("mkdir -p /root/build", logch, nil)
				if err != nil {
					errch <- err
					return
				}
				//install packages
				if pk.Builder == "bootstrap" {
					err = sb.RunCmd(fmt.Sprintf("apk --no-cache add make bash gcc libc-dev binutils ccache %s", strings.Join(pk.BuildDependencies, " ")), logch, nil)
					if err != nil {
						errch <- err
						return
					}
				} else {
					//create dir for packages
					err = sb.RunCmd("mkdir -p /root/pkgs", logch, nil)
					if err != nil {
						errch <- err
						return
					}
					//create a package loader
					pl := &ploader{
						sb:     sb,
						loaded: make(map[string]bool),
						loader: func(name string) (io.ReadCloser, error) {
							sn, sr := r.Stream(0)
							w.Send(bmapi.PackageRequestMessage{
								Name:   name,
								Stream: sn,
							})
							return sr, nil
						},
						logch:  logch,
						lorder: []string{},
					}
					//load base package
					err = pl.load("base-build")
					if err != nil {
						errch <- err
						return
					}
					//load build dependencies
					for _, v := range pk.BuildDependencies {
						err = pl.load(v)
						if err != nil {
							errch <- err
							return
						}
					}
					//make lpkg database directory
					err = sb.RunCmd("mkdir -p /etc/lpkg.d/db", logch, nil)
					if err != nil {
						errch <- err
						return
					}
					//untar lpkg
					err = sb.RunCmd("tar -xf /root/pkgs/lpkg.tar.gz -C / .", logch, nil)
					if err != nil {
						errch <- err
						return
					}
					//use lpkg to install packages
					err = sb.RunCmd(fmt.Sprintf("for %s in /root/pkgs/*.tar.gz; do lpkg-inst $i; done", strings.Join(pl.lorder, " ")), logch, nil)
					if err != nil {
						errch <- err
						return
					}
				}
				//generate Makefile
				{
					mf := pk.GenFullMakefile(pkgen.DefaultVars)
					//write Makefile to buffer
					mfbuf := bytes.NewBuffer(nil)
					_, err = mf.WriteTo(mfbuf)
					if err != nil {
						errch <- err
						return
					}
					//dump buffer onto server
					err = sb.WriteFile("/root/build/Makefile", mfbuf)
					if err != nil {
						errch <- err
						return
					}
				}
				//create sources dir
				err = sb.RunCmd("mkdir -p /root/build/src", logch, nil)
				if err != nil {
					errch <- err
					return
				}
				//create loader
				loader, err := pkgen.NewMultiLoader(
					bmapi.NewFileRequestLoader(w, r),
					dload,
				)
				if err != nil {
					errch <- err
					return
				}
				//generate source tar to server
				piper, pipew := io.Pipe()
				go func() {
					pipew.CloseWithError(pk.WriteSourceTar(pipew, loader, 100*1024*1024))
				}()
				err = sb.WriteFile("/root/build/src.tar", piper)
				if cerr := piper.Close(); cerr != nil {
					if err != nil {
						err = cerr
					}
				}
				if err != nil {
					errch <- err
					return
				}
				//execute Makefile
				err = sb.RunCmd("make -j10 -l6 -C /root/build", logch, nil)
				if err != nil {
					errch <- err
					return
				}
				//send list of packages
				err = w.Send(bmapi.PackagesReadyMessage(pk.ListPackages()))
				if err != nil {
					errch <- err
					return
				}
				//ayyy we are done!!!
				//now send packages back
				//process package requests
				for pr := range pkrch {
					srw := w.Stream(pr.Stream)
					err = sb.ReadFile(fmt.Sprintf("/root/pkgs/tars/%s.tar.gz", pr.Name), srw)
					if err != nil {
						errch <- err
						return
					}
				}
			}()
			for {
				select {
				case m := <-rch:
					switch mv := m.(type) {
					case bmapi.ErrorMessage:
						go func() {
							defer droperr()
							errch <- mv
						}()
					case bmapi.DatMessage, bmapi.StreamDoneMessage:
						err := r.HandleDat(mv)
						if err != nil {
							return err
						}
					case bmapi.PackageRequestMessage:
						go func() {
							defer droperr()
							pkrch <- mv
						}()
					case bmapi.DoneMessage:
						return nil
					}
				case err = <-errch:
					w.Send(bmapi.ErrorMessage(err.Error()))
					return err
				}
			}
		})
	}))
}

type sshBot struct {
	cli *ssh.Client
}

func (sb *sshBot) ReadFile(path string, dest io.Writer) (err error) {
	//create ssh session
	sess, err := sb.cli.NewSession()
	if err != nil {
		return
	}
	defer func() {
		cerr := sess.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	//setup stdout
	sess.Stdout = dest
	//run cat
	err = sess.Run(fmt.Sprintf("cat %q", path))
	return
}

//Write to file
func (sb *sshBot) WriteFile(path string, src io.Reader) (err error) {
	//create ssh session
	sess, err := sb.cli.NewSession()
	if err != nil {
		return
	}
	defer func() {
		cerr := sess.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	//setup stdout
	sess.Stdin = src
	//run cat
	err = sess.Run(fmt.Sprintf("cat > %q", path))
	return
}

func streamlog(src io.Reader, logout chan<- bmapi.LogMessage, streamn uint8) {
	defer recover()
	s := bufio.NewScanner(src)
	for s.Scan() {
		logout <- bmapi.LogMessage{
			Text:   s.Text(),
			Stream: streamn,
		}
	}
}

func (sb *sshBot) RunCmd(cmd string, logout chan<- bmapi.LogMessage, stdin io.Reader) (err error) {
	//create ssh session
	sess, err := sb.cli.NewSession()
	if err != nil {
		return
	}
	defer func() {
		cerr := sess.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	//allow using user provided stdin
	sess.Stdin = stdin
	//set up logging
	or, err := sess.StdoutPipe()
	if err != nil {
		return
	}
	er, err := sess.StderrPipe()
	if err != nil {
		return
	}
	go streamlog(or, logout, 1)
	go streamlog(er, logout, 2)
	//run command
	err = sess.Run(cmd)
	return
}

//RunSimple runs a command and writes output to the writer
//writes stderr to logout
func (sb *sshBot) RunSimple(cmd string, out io.Writer, logout chan<- bmapi.LogMessage) (err error) {
	//create ssh session
	sess, err := sb.cli.NewSession()
	if err != nil {
		return
	}
	defer func() {
		cerr := sess.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	//set output writer
	sess.Stdout = out
	//set up logging
	er, err := sess.StderrPipe()
	if err != nil {
		return
	}
	go streamlog(er, logout, 2)
	//run run command
	err = sess.Run(cmd)
	return
}

type ploader struct {
	sb     *sshBot
	loaded map[string]bool
	loader func(string) (io.ReadCloser, error)
	logch  chan<- bmapi.LogMessage
	lorder []string
}

func (pl *ploader) load(pkname string) (err error) {
	//dont double load
	if pl.loaded[pkname] {
		return
	}
	pl.loaded[pkname] = true
	//set up values
	pkf := fmt.Sprintf("/root/pkgs/%s.tar.gz", pkname)
	//get a reader and copy to server
	err = func() (err error) {
		r, err := pl.loader(pkname)
		if err != nil {
			return
		}
		defer func() {
			cerr := r.Close()
			if cerr != nil {
				if err != nil {
					err = cerr
				}
			}
		}()
		err = pl.sb.WriteFile(pkf, r)
		if err != nil {
			return
		}
		return
	}()
	if err != nil {
		return
	}
	//read package dependencies
	pb := bytes.NewBuffer(nil)
	err = pl.sb.RunSimple(fmt.Sprintf("tar -xOf %s ./.pkginfo | (source /dev/stdin && echo -n \"$DEPENDENCIES\")", pkf), pb, pl.logch)
	if err != nil {
		return err
	}
	deps := strings.Split(pb.String(), " ")
	//load dependencies
	for _, d := range deps {
		err = pl.load(d)
		if err != nil {
			return
		}
	}
	//add to load order
	pl.lorder = append(pl.lorder, pkname)
	return
}
