package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
)

type ID int

const (
	REG ID = iota
	JOIN
	LEAVE
	MSG
	CHNS
	USRS
)

type command struct {
	id        ID
	recipient string
	sender    string
	body      []byte
}

type client struct {
	conn       net.Conn
	outbound   chan<- command
	register   chan<- *client
	deregister chan<- *client
	username   string
}

func newClient(
	conn net.Conn,
	cmds chan<- command,
	registrations chan<- *client,
	deregistrations chan<- *client,
) *client {
	return &client{
		conn:       conn,
		outbound:   cmds,
		register:   registrations,
		deregister: deregistrations,
	}
}

func (c *client) read() error {
	for {
		msg, err := bufio.NewReader(c.conn).ReadBytes('\n')
		if err == io.EOF {
			c.deregister <- c
			return nil
		}
		if err != nil {
			return err
		}
		c.handle(msg)
	}
}

func (c *client) handle(message []byte) {
	cmd := bytes.ToUpper(bytes.TrimSpace(bytes.Split(message, []byte(" "))[0]))
	args := bytes.TrimSpace(bytes.TrimPrefix(message, cmd))

	switch string(cmd) {
	case "REG":
		if err := c.reg(args); err != nil {
			c.err(err)
		}
	case "JOIN":
		if err := c.join(args); err != nil {
			c.err(err)
		}
	// case "LEAVE":
	// 	if err := c.leave(args); err != nil {
	// 		c.err(err)
	// 	}
	case "MSG":
		if err := c.msg(args); err != nil {
			c.err(err)
		}
	// case "CHNS":
	// 	c.chns()
	// case "USRS":
	// 	c.usrs()
	default:
		c.err(fmt.Errorf("unknown command %s", cmd))
	}
}

func (c *client) reg(args []byte) error {
	fmt.Println(string(args))
	u := bytes.TrimSpace(args)
	if u[0] != '@' {
		return fmt.Errorf("username must start with @")
	}
	if len(u) == 0 {
		return fmt.Errorf("username must not be empty")
	}
	c.username = string(u)
	c.register <- c
	return nil
}

func (c *client) join(args []byte) error {
	// JOIN #general
	args = bytes.TrimSpace(args)
	if args[0] != '#' {
		return fmt.Errorf("channel name must start with #")
	}
	if len(args) == 0 {
		return fmt.Errorf("channel name must not be empty")
	}
	c.outbound <- command{
		recipient: string(args),
		sender:    c.username,
		id:        JOIN,
		body:      []byte(c.username + " has joined the channel"),
	}
	return nil
}

// func (c *client) leave(args []byte) error {}
func (c *client) msg(args []byte) error {
	// MSG #general 6\r\nHello!
	// MSG @jane 4\r\nHey!

	DELIMITER := []byte{}
	DELIMITER = append(DELIMITER, ' ')
	args = bytes.TrimSpace(args)
	if args[0] != '#' && args[0] != '@' {
		return fmt.Errorf("recipient must start with # or @")
	}
	recipient := bytes.Split(args, []byte(" "))[0]
	if len(recipient) == 0 {
		return fmt.Errorf("recipient must have a name")
	}
	args = bytes.TrimSpace(bytes.TrimPrefix(args, recipient))
	fmt.Println(string(args))
	fmt.Println(string(DELIMITER))
	l := bytes.Split(args, DELIMITER)[0]
	fmt.Println(string(l))
	length, err := strconv.Atoi(string(l))
	fmt.Println(err)
	if err != nil {
		return fmt.Errorf("body length must be present")
	}
	if length == 0 {
		return fmt.Errorf("body length must be at least 1")
	}
	padding := len(l) + len(DELIMITER)
	body := args[padding : padding+length]
	c.outbound <- command{
		recipient: string(recipient),
		sender:    c.username,
		body:      body,
		id:        MSG,
	}
	return nil
}

func (c *client) err(e error) {
	c.conn.Write([]byte("ERR " + e.Error() + "\n"))
}

type hub struct {
	channels        map[string]*channel
	clients         map[string]*client
	commands        chan command
	deregistrations chan *client
	registrations   chan *client
}

func newHub() *hub {
	return &hub{
		channels:        make(map[string]*channel),
		clients:         make(map[string]*client),
		commands:        make(chan command),
		deregistrations: make(chan *client),
		registrations:   make(chan *client),
	}
}

func (h *hub) run() {
	for {
		select {
		case client := <-h.registrations:
			h.register(client)
		case client := <-h.deregistrations:
			h.unregister(client)
		case cmd := <-h.commands:
			switch cmd.id {
			case JOIN:
				h.joinChannel(cmd.sender, cmd.recipient)
			// case LEAVE:
			// h.leaveChannel(cmd.sender, cmd.recipient)
			case MSG:
				h.message(cmd.sender, cmd.recipient, cmd.body)
			// case USRS:
			// 	h.listUsers(cmd.sender)
			// case CHNS:
			// 	h.listChannels(cmd.sender)
			default:
			}
		}
	}
}

func (h *hub) register(c *client) {
	if _, exists := h.clients[c.username]; exists {
		c.username = ""
		c.conn.Write([]byte("ERR username taken\n"))
	} else {
		h.clients[c.username] = c
		c.conn.Write([]byte("OK\n"))
	}
}

func (h *hub) unregister(c *client) {
	if _, exists := h.clients[c.username]; exists {
		delete(h.clients, c.username)
		for _, channel := range h.channels {
			delete(channel.clients, c)
		}
	}
}

func (h *hub) message(u string, r string, m []byte) {
	if sender, ok := h.clients[u]; ok {
		switch r[0] {
		case '#':
			if channel, ok := h.channels[r]; ok {
				if _, ok := channel.clients[sender]; ok {
					channel.broadcast(sender.username, m)
				}
			}
		case '@':
			if user, ok := h.clients[r]; ok {
				user.conn.Write(append(m, '\n'))
			}
		}
	}
}

func (h *hub) joinChannel(u string, c string) {
	if client, ok := h.clients[u]; ok {
		if channel, ok := h.channels[c]; ok {
			channel.clients[client] = true
		} else {
			h.channels[c] = newChannel(c)
			h.channels[c].clients[client] = true
		}
		client.conn.Write([]byte("OK\n"))
	}
}

type channel struct {
	name    string
	clients map[*client]bool
}

func newChannel(c string) *channel {
	return &channel{
		name:    c,
		clients: make(map[*client]bool),
	}
}

func (c *channel) broadcast(s string, m []byte) {
	msg := append([]byte(s), ": "...)
	msg = append(msg, m...)
	msg = append(msg, '\n')

	for cl := range c.clients {
		cl.conn.Write(msg)
	}
}

func main() {
	ln, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Printf("%v", err)
	}

	hub := newHub()
	go hub.run()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("%v", err)
		}

		c := newClient(
			conn,
			hub.commands,
			hub.registrations,
			hub.deregistrations,
		)

		go c.read()
	}
}
