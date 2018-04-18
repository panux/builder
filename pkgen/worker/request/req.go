package request

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
)

//APIVersion is the current API version
var APIVersion = uint(1)

//Request is the main request type
type Request struct {
	//APIVersion is the version of the API (required)
	APIVersion uint `json:"apiVersion"`

	//Request is the underlying request (specific to type of request)
	//required except for a status request
	Request interface{} `json:"request,omitempty"`

	//PublicKey is the public key used to sign the request
	//Stored in PKCS1 format
	PublicKey []byte
}

//Sig is the JSON of a signed request
type Sig struct {
	Dat       []byte `json:"dat"`
	Key       []byte `json:"pubkey"` //PKCS1 format
	Signature []byte `json:"sig"`
}

//Sign converts the request to JSON and signs it, returning the signed data
//NOTE: modifies PublicKey field
func (r *Request) Sign(privkey *rsa.PrivateKey) ([]byte, error) {
	//encode public key
	r.PublicKey = x509.MarshalPKCS1PublicKey(&privkey.PublicKey)

	//marshal Request JSON
	dat, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	//sign JSON
	hash := sha256.Sum256(dat)
	sig, err := rsa.SignPKCS1v15(rand.Reader, privkey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, err
	}

	//marshal Sig
	rdat, err := json.Marshal(Sig{
		Dat:       dat,
		Signature: sig,
	})
	if err != nil {
		return nil, err
	}

	return rdat, nil
}

//ErrKeyMismatch is an error returned when the Request.PublicKey does not match the Sig.Key
var ErrKeyMismatch = errors.New("request key does not match signature key")

//DecodeRequest decodes a request
//reqsub should be an appropriate container for the Request field of the Request
func DecodeRequest(raw string, reqsub interface{}) (*Request, error) {
	//unmarshal Sig
	var sig Sig
	err := json.Unmarshal([]byte(raw), &sig)
	if err != nil {
		return nil, err
	}

	//unmarshal public key
	pubkey, err := x509.ParsePKCS1PublicKey(sig.Key)
	if err != nil {
		return nil, err
	}

	//validate signature
	mhash := sha256.Sum256(sig.Dat)
	err = rsa.VerifyPKCS1v15(pubkey, crypto.SHA256, mhash[:], sig.Signature)
	if err != nil {
		return nil, err
	}

	//unmarshal Request
	req := new(Request)
	req.Request = reqsub
	err = json.Unmarshal(sig.Dat, req)
	if err != nil {
		return nil, err
	}

	//check for key mismatch
	if !bytes.Equal(req.PublicKey, sig.Key) {
		return nil, ErrKeyMismatch
	}

	return req, nil
}
