package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// const es = new EventSource("http://localhost:4040/event-stream")
// es.addEventListener("greetings", function( e ) {
// 	console.log(e)
// }, false);

type Agent struct {
	sync.Mutex
	clients   map[string]*client
	closeChan chan struct{}
}

func (cr *Agent) registerClient() *client {
	c := client{
		id:        uuid.NewString(),
		closeChan: make(chan struct{}),
		eventChan: make(chan Event),
	}
	cr.Lock()
	defer cr.Unlock()
	cr.clients[c.id] = &c
	return &c
}

func (a *Agent) remove(c *client) {
	a.Lock()
	defer a.Unlock()
	delete(a.clients, c.id)
}

type client struct {
	id        string
	details   string
	eventChan chan Event
	closeChan chan struct{}
}

func (c *client) close() {
	c.closeChan <- struct{}{}
}

func (a *Agent) start() {
	log.Println("Supervisor started")
	<-a.closeChan
	log.Println("Got close msg")
	a.Lock()
	defer a.Unlock()
	log.Println("Lock applied")
	for _, c := range agent.clients {
		log.Printf("drop client %s", c.id)
		go agent.dropClient(c)
	}
	log.Println("Supervisor stopped")
}

func (a *Agent) close() {
	fmt.Println("close agent")
	a.closeChan <- struct{}{}
	fmt.Println("close sent")
}

func (a *Agent) dropClient(c *client) {
	fmt.Println("send close to client", c.id)
	c.close()
	fmt.Println("remove from registry", c.id)
	a.remove(c)
	fmt.Println("removed from registry", c.id)
}

type Event struct {
	id    string
	event string
	data  string
}

func (e Event) String() string {
	return fmt.Sprintf("id:%s\nevent:%s\ndata:%s\n\n", e.id, e.event, e.data)
}

func (a *Agent) CreateSseEvent(id, event string, data any) (*Event, error) {
	d, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Event{
		id:    id,
		event: event,
		data:  string(d),
	}, nil
}

func (a *Agent) broadcast(e *Event) {
	a.Lock()
	defer a.Unlock()
	for _, c := range a.clients {
		go func(ch chan<- Event) {
			ch <- *e
		}(c.eventChan)
	}
}

func (a *Agent) handleSseRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Cache-Control", "no-cache")
	w.Header().Add("Connection", "keep-alive")
	w.Header().Add("Content-Type", "text/event-stream")

	c := a.registerClient()
	log.Println("New registered connection", c.id)

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Unable to establish persistence connection", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case e := <-c.eventChan:
			_, err := io.WriteString(w, e.String())
			if err != nil {
				log.Printf("Error at sending message to client: %s, error: %s", c.id, err.Error())
				agent.dropClient(c)
				return
			}
			f.Flush()

		case <-r.Context().Done():
			log.Printf("Client closed connection: %s", c.id)
			agent.remove(c)
			return
		case <-c.closeChan:
			// w.WriteHeader(http.StatusGone)
			// w.Write(nil)
			// f.Flush()
			log.Printf("Server closed connenction for Client: %s", c.id)
			return
		}
	}
}

var agent = Agent{
	Mutex:     sync.Mutex{},
	clients:   map[string]*client{},
	closeChan: make(chan struct{}),
}

type TestMsg struct {
	ID          int64
	Name        string
	Description string
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("Hello"))
	})
	http.HandleFunc("/event-stream", agent.handleSseRequest)
	http.HandleFunc("/send", func(w http.ResponseWriter, _ *http.Request) {
		event, err := agent.CreateSseEvent(
			"#1",
			"greetings",
			&TestMsg{
				ID:          12,
				Name:        "A. Sz.",
				Description: "Hello!\n\n. Good Bye!\n",
			})

		agent.broadcast(event)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	http.HandleFunc("/finish", func(_ http.ResponseWriter, _ *http.Request) {
		agent.close()
	})
	http.HandleFunc("/start", func(_ http.ResponseWriter, _ *http.Request) {
		go agent.start()
	})
	log.Fatal(http.ListenAndServe(":4040", nil))
}
