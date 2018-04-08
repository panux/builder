package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os/exec"
	"syscall"

	"golang.org/x/crypto/ssh"
)

func main() {
	var authkeypath string
	var srvkeypath string
	var listenaddr string
	flag.StringVar(&authkeypath, "authorizedkeys", "authorized_keys", "authorized_keys file")
	flag.StringVar(&srvkeypath, "srvkey", "", "path to SSH server key")
	flag.StringVar(&listenaddr, "ssh", ":20", "listening address for SSH server")
	flag.Parse()
	//load public keys
	authmap := make(map[string]bool)
	{
		dat, err := ioutil.ReadFile(authkeypath)
		if err != nil {
			log.Fatalf("Failed to load authorizedkeys (%q), err: %q\n", authkeypath, err.Error())
		}
		for len(dat) > 0 {
			pk, _, _, left, err := ssh.ParseAuthorizedKey(dat)
			if err != nil {
				log.Fatalf("Failed to parse authkey: %q\n", err.Error())
			}
			authmap[string(pk.Marshal())] = true
			dat = left
		}
	}
	//create config with pubkey auth
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pk ssh.PublicKey) (*ssh.Permissions, error) {
			if authmap[string(pk.Marshal())] {
				return &ssh.Permissions{
					Extensions: map[string]string{
						"pubkey-fp": ssh.FingerprintSHA256(pk),
					},
				}, nil
			}
			log.Printf("Unauthorized access attempt by %q\n", c.RemoteAddr().String())
			return nil, errors.New("Authentication failure")
		},
	}
	//load server key
	{
		dat, err := ioutil.ReadFile(srvkeypath)
		if err != nil {
			log.Fatalf("Failed to load private key: %q\n", err.Error())
		}
		pkey, err := ssh.ParsePrivateKey(dat)
		if err != nil {
			log.Fatalf("Failed to parse private key: %q\n", err.Error())
		}
		cfg.AddHostKey(pkey)
	}
	//start listening
	l, err := net.Listen("tcp", listenaddr)
	if err != nil {
		log.Fatalf("Failed to listen: %q\n", err.Error())
	}
	//connection handling loop
	for {
		c, err := l.Accept()
		if err != nil {
			log.Printf("Failed to accept: %q\n", err.Error())
		}
		go func() { //process connection
			conn, chans, reqs, err := ssh.NewServerConn(c, cfg)
			if err != nil {
				log.Fatalf("Failed to create ssh server conn: %q\n", err.Error())
			}
			log.Printf("Login with key %s\n", conn.Permissions.Extensions["pubkey-fp"])
			go ssh.DiscardRequests(reqs) //we dont use these
			for ch := range chans {
				switch ch.ChannelType() {
				case "session":
					sch, reqs, err := ch.Accept()
					if err != nil {
						log.Printf("Failed to accept session channel: %q\n", err.Error())
						continue
					}
					go func() { //process requests
						defer func() { //handle channel cleanup
							err := sch.Close()
							if err != nil {
								log.Printf("Close error on session channel: %q\n", err.Error())
							}
						}()
						for req := range reqs {
							switch req.Type {
							case "exec":
								shstr := string(req.Payload)
								shstr = shstr[4:]
								//please build busybox with CONFIG_FEATURE_SH_STANDALONE
								cmd := exec.Command("busybox", "sh", "-c", shstr)
								cmd.Stdin = sch
								cmd.Stdout = sch
								cmd.Stderr = sch.Stderr()
								err := cmd.Run()
								var exitcode syscall.WaitStatus
								if err != nil {
									ee, ok := err.(*exec.ExitError)
									if ok {
										exitcode = ee.Sys().(syscall.WaitStatus)
									} else {
										log.Printf("Command failed to execute: %q\n", err.Error())
										return
									}
								}
								var buf bytes.Buffer
								err = binary.Write(&buf, binary.BigEndian, exitcode>>8)
								if err != nil {
									log.Fatalf("Failed to encode exit code: %q\n", err.Error())
								}
								_, err = sch.SendRequest("exit-status", false, buf.Bytes())
								if err != nil {
									log.Fatalf("Failed to send exit-status request: %q\n", err.Error())
								}
								return
							}
						}
					}()
				default:
					ch.Reject(ssh.UnknownChannelType, "channel type not supported")
				}
			}
		}()
	}
}
