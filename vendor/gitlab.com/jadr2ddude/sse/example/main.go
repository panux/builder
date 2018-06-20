package main

import (
	"fmt"
	"log"
	"net/http"

	"gitlab.com/jadr2ddude/sse"
)

type msg struct {
	User string `json:"user"`
	Text string `json:"text"`
}

var connect = make(chan chan<- msg)
var disconnect = make(chan chan<- msg)
var message = make(chan msg, 2)

func main() {
	http.Handle("/", http.FileServer(http.Dir(".")))
	http.Handle("/event", sse.Handler(handleEvent))
	http.HandleFunc("/send", handleSend)
	go doChat()
	http.ListenAndServe(":8081", nil)
}

func doChat() {
	listeners := map[chan<- msg]bool{}
	for {
		select {
		case ch := <-connect:
			listeners[ch] = true
		case ch := <-disconnect:
			delete(listeners, ch)
		case m := <-message:
			log.Println(m)
			for l := range listeners {
			fwd:
				select {
				case l <- m:
				case ch := <-disconnect:
					delete(listeners, ch)
					if ch != l {
						goto fwd
					}
				}
			}
		}
	}
}

func handleEvent(ss *sse.Sender, r *http.Request) {

	//create message channel
	ch := make(chan msg, 2)
	defer close(ch)

	//connect
	connect <- ch
	defer func() { disconnect <- ch }()

	err := ss.SendJSON(msg{
		User: "server",
		Text: "connected",
	})
	if err != nil {
		return
	}

	//send messages
	for m := range ch {
		err = ss.SendJSON(m)
		if err != nil {
			return
		}
	}
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(100 * 1024)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse form: %q", err.Error()), http.StatusBadRequest)
		return
	}
	message <- msg{r.FormValue("user"), r.FormValue("text")}
}
