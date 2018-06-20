package sse

import "net/http"

//Handler is an http.Handler which uses SSE.
//If there is an error in NewSender, an http.StatusInternalServerError is sent.
type Handler func(*Sender, *http.Request)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s, err := NewSender(w)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h(s, r)
}
