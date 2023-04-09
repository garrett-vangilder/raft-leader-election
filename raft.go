package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	zmq "github.com/pebbe/zmq4"
)

type Peer struct {
	name    string `json:"name"`
	address string `json:"address"`
}

type RaftNode struct {
	name          string      // The ID of the node.
	address       string      // The address of the node.
	port          string      // The port of the node.
	currentTerm   int         // The latest term the node has seen.
	votedFor      string      // The ID of the node the current node has voted for in this term.
	state         string      // The state of the node (follower, candidate, or leader).
	electionTimer *time.Timer // The timer used to trigger a new election.

	mutex        sync.Mutex
	dealerSocket *zmq.Socket
	recvSocket   *zmq.Socket
	poller       *zmq.Poller

	// reference to peer addresses
	peers []Peer
}

// Message represents a message sent between nodes in the Raft cluster.
type Message struct {
	Type        string `json:"type"`
	Term        int    `json:"term"`
	From        string `json:"from"`
	To          string `json:"to"`
	Command     string `json:"command"`
	VoteGranted bool   `json:"voteGranted"`
}

func (n *RaftNode) connectToPeers() {
	fmt.Println("raft::connectToPeers()")

	// Create a router socket and bind it to address.
	fmt.Println("raft::connectToPeers() - creating router socket")
	router, err := zmq.NewSocket(zmq.ROUTER)
	router.Bind("tcp://" + n.address + ":" + n.port)

	if err != nil {
		fmt.Println("raft::connectToPeers() - failed to create new socket: ", err)
	}

	// create poller
	poller := zmq.NewPoller()
	if err != nil {
		log.Fatal(err)
	}

	poller.Add(router, zmq.POLLIN)

	// // create socket
	dealer, err := zmq.NewSocket(zmq.DEALER)
	if err != nil {
		log.Fatal(err)
	}
	dealer.Connect("tcp://" + n.address + ":" + n.port)

	// add dealer to poller
	poller.Add(dealer, zmq.POLLIN)

	// // connect to peers
	for _, peer := range n.peers {
		fmt.Println("Connecting to peer: ", peer)
		router.Bind("tcp://" + peer.address)
	}

	n.recvSocket = router
	n.dealerSocket = dealer
	n.poller = poller
}

func (n *RaftNode) listenForResponses() {
	fmt.Printf("Listening for responses on %s...\n", n.address)
	for {
		// declare message
		var msg []string

		// read from poller
		sockets, _ := n.poller.Poll(-1)

		for _, socket := range sockets {
			switch s := socket.Socket; s {
			case n.dealerSocket:
				fmt.Println("Received message from dealerSocket")
				_msg, err := s.RecvMessage(0)
				if err != nil {
					fmt.Println("Error receiving message:", err)
					continue
				}
				msg = _msg
			case n.recvSocket:
				fmt.Println("Received message from recvSocket")
				_msg, err := s.RecvMessage(0)
				if err != nil {
					fmt.Println("Error receiving message:", err)
					continue
				}
				msg = _msg
			}

			// decode message
			var message Message
			err := json.Unmarshal([]byte(msg[1]), &message)
			if err != nil {
				fmt.Println("Error decoding message:", err)
				continue
			}

			// print the message
			fmt.Println("raft::listenForResponses() - message type: ", message.Type)
			fmt.Println("raft::listenForResponses() - message term: ", message.Term)
			fmt.Println("raft::listenForResponses() - message from: ", message.From)
			fmt.Println("raft::listenForResponses() - message to: ", message.To)

			// drop the message if its from us
			if message.To == n.name {
				n.receiveMessage(message)
			}
		}
	}
}

func (n *RaftNode) receiveMessage(message Message) {
	fmt.Println("raft::receiveMessage()")
	fmt.Println("raft::receiveMessage() - message type: ", message.Type)

	switch n.state {
	case "follower":
		if message.Type == "RequestVote" {
			fmt.Println("raft::receiveMessage() - message type is RequestVote")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// check if message term is greater than current term
			if message.Term > n.currentTerm {
				fmt.Println("raft::receiveMessage() - message term is greater than current term")
				fmt.Println("raft::receiveMessage() - becoming follower")
				n.becomeFollower(message.Term)

				// check if we have already voted for someone else
				if n.votedFor != "" {
					fmt.Println("raft::receiveMessage() - already voted for someone else")
					return
				}

				// grant vote
				fmt.Println("raft::receiveMessage() - granting vote")
				n.votedFor = message.From
				n.sendVoteResponse(message.From, true)
			}
		} else if message.Type == "Heartbeat" {
			fmt.Println("raft::receiveMessage() - message type is Heartbeat")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// check if message term is greater than current term
			if message.Term > n.currentTerm {
				fmt.Println("raft::receiveMessage() - message term is greater than current term")
				fmt.Println("raft::receiveMessage() - becoming follower")
				n.becomeFollower(message.Term)
			}
		}
	case "candidate":
		// If the message is a vote request, grant or deny the vote.
		// If the message is a heartbeat from the leader with a higher term, become a follower.
		{
		}
	case "leader":
		// If the message is a vote request, grant or deny the vote.
		// If the message is a heartbeat from the leader with a higher term, become a follower.
		// If the message is a heartbeat from the leader with the same term, reset the election timer.
		{
		}
	}
}

func getPeersFromConfig(pathToConfig string) []Peer {
	// TODO This is broken and does not work
	data, err := ioutil.ReadFile(pathToConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Convert the byte slice to a string
	jsonString := string(data)

	// Print the JSON string to the console
	fmt.Println(jsonString)

	var peers []Peer

	err = json.Unmarshal([]byte(data), &peers)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println((peers))

	return peers
}

// startElectionTimer starts the election timer with a random timeout between 150ms and 300ms.
func (n *RaftNode) startElectionTimer() {
	timeout := time.Duration(rand.Intn(150)+150) * time.Millisecond
	n.electionTimer = time.NewTimer(timeout)
}

// becomeCandidate transitions the node to the candidate state and starts a new election.
func (n *RaftNode) becomeCandidate() {
	// fmt.Println("Becoming candidate")
	n.state = "candidate"
	n.currentTerm++
	n.votedFor = n.name
	n.startElectionTimer()
}

func (n *RaftNode) becomeFollower(term int) {
	n.state = "follower"
	n.currentTerm = term
	n.startElectionTimer()
}

// requestVote sends a vote request to the given node and returns true if the vote is granted.
func (n *RaftNode) requestVote() {
	fmt.Println("raft::requestVote()")

	// Increment the current term and vote for ourselves.
	fmt.Println("raft::requestVote() - incrementing current term")
	n.currentTerm++

	// setting votedFor to self
	fmt.Println("raft::requestVote() - setting votedFor to self")
	n.votedFor = n.name

	// Create the request message.
	// send to each peer
	for _, peer := range n.peers {
		fmt.Println("raft::requestVote() - sending request to: ", peer)
		request := Message{
			Type:    "RequestVote",
			Term:    n.currentTerm,
			From:    n.name,
			To:      peer.name,
			Command: "",
		}

		// Encode the request message as JSON.
		payload, err := json.Marshal(request)
		if err != nil {
			fmt.Println("Error encoding message:", err)
			return
		}

		fmt.Println("Sending message")
		_, err = n.dealerSocket.SendBytes(payload, 0)
		if err != nil {
			fmt.Println("Error sending message:", err)
			return
		}
		fmt.Println("Message sent")

	}
}

// sendVoteResponse sends a vote response to the given node.
func (n *RaftNode) sendVoteResponse(to string, voteGranted bool) {
	fmt.Println("raft::sendVoteResponse()")
	fmt.Println("raft::sendVoteResponse() - to: ", to)
	// Create the response message.
	response := Message{
		Type:        "RequestVoteResponse",
		Term:        n.currentTerm,
		From:        n.name,
		To:          to,
		Command:     "",
		VoteGranted: voteGranted,
	}

	// Encode the response message as JSON.
	payload, err := json.Marshal(response)
	if err != nil {
		fmt.Println("Error encoding message:", err)
		return
	}

	fmt.Println("Sending message")
	_, err = n.dealerSocket.SendBytes(payload, 0)
	if err != nil {
		fmt.Println("Error sending message:", err)
		return
	}

	fmt.Println("Message sent")
}

func (n *RaftNode) run() {
	// Start the election timer.
	n.startElectionTimer()

	for {
		// sleep for 1 second
		time.Sleep(1 * time.Second)

		switch n.state {
		case "follower":
			// Wait for a message from the leader or a candidate.
			// If the election timer expires, become a candidate.
			select {
			case <-n.electionTimer.C:
				n.becomeCandidate()
			}
		case "candidate":
			// Send vote requests to all other nodes.
			n.requestVote()

			// If a majority of nodes vote for this node, become the leader.

			// If a leader is elected, change state and return.
			// If the election timer expires, start a new election.
			// If a vote request or heartbeat message is received from the leader, become a follower.
		case "leader":
			// Send heartbeat messages to all other nodes.
			// If a node does not receive a heartbeat within the election timeout, start a new election.
			// If a node receives a higher term from a message, become a follower.
		}
	}
}

func newRaftNode(name string, address string, port string, peers []Peer) *RaftNode {
	return &RaftNode{
		name:    name,
		address: address,
		port:    port,
		state:   "follower",
		peers:   peers,
	}
}

func main() {
	fmt.Println("raft::main")

	// parse command line arguments
	if len(os.Args) != 4 {
		fmt.Println("Usage <node name>")
		os.Exit(1)
	}
	name := os.Args[1]
	fmt.Println("raft::main - name: ", name)

	// address is second argument
	address := os.Args[2]
	fmt.Println("raft::main - address: ", address)

	// port is third argument
	port := os.Args[3]
	fmt.Println("raft::main - port: ", port)

	// parse config from json
	peers := getPeersFromConfig("config.json")

	// remove self from peers
	for i, peer := range peers {
		if peer.address == address+":"+port {
			peers = append(peers[:i], peers[i+1:]...)
			break
		}
	}
	fmt.Println("raft::main - peers: ", peers)

	node := newRaftNode(name, address, port, peers)
	return
	fmt.Println("raft::main - created new node: ", node.name)

	// connect node to peers
	node.connectToPeers()

	// Start listening for responses on the ZeroMQ socket.
	go node.listenForResponses()

	// Start the process
	go node.run()

	// Wait for input to exit.
	var input string
	fmt.Scanln(&input)

	// cleanup sockets
	defer node.dealerSocket.Close()
	defer node.recvSocket.Close()
}
