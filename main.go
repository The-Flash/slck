package main

import (
	"bufio"
	"io"
	"net"
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
	body      []string
}

type client struct {
	conn       net.Conn
	outbound   chan<- command
	register   chan<- *client
	deregister chan<- *client
	username   string
}

type channel struct {
	name    string
	clients map[*client]bool
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

func (c *channel) broadcast(s string, m []byte) {
	msg := append([]byte(s), ": "...)
	msg = append(msg, m...)
	msg = append(msg, '\n')

	for cl := range c.clients {
		cl.conn.write(msg)
	}
}

func main() {

}
