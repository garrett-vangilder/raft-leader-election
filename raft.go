package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/zeromq/goczmq"
)

type RaftNode struct {
	name          string      // The ID of the node.
	address       string      // The address of the node.
	currentTerm   int         // The latest term the node has seen.
	votedFor      string      // The ID of the node the current node has voted for in this term.
	state         string      // The state of the node (follower, candidate, or leader).
	electionTimer *time.Timer // The timer used to trigger a new election.

	mutex        sync.Mutex
	dealerSocket *goczmq.Sock
	recvSocket   *goczmq.Sock

	// reference to peer addresses
	peers []string
}

// Message represents a message sent between nodes in the Raft cluster.
type Message struct {
	Type    string `json:"type"`
	Term    int    `json:"term"`
	From    string `json:"from"`
	To      string `json:"to"`
	Command string `json:"command"`
}

func (n *RaftNode) connectToPeers() {
	// Create a router socket and bind it to address.
	fmt.Println("Creating router at tcp://" + n.address + "...")
	router, err := goczmq.NewRouter("tcp://" + n.address)
	if err != nil {
		log.Fatal(err)
	}
	n.recvSocket = router

	fmt.Println("Router created at tcp://" + n.address)

	// create socket
	dealer, err := goczmq.NewDealer("tcp://127.0.0.1:5555")
	if err != nil {
		log.Fatal(err)
	}
	n.dealerSocket = dealer

	// connect to peers
	for _, peer := range n.peers {
		fmt.Println("Connecting to peer: ", peer)
		n.dealerSocket.Connect(peer)
	}
}

func (n *RaftNode) listenForResponses() {
	fmt.Printf("Listening for responses on %s...\n", n.address)
	for {
		fmt.Println("n.recvSocket.RecvMessage()")
		fmt.Println("n.recvSocket: ", n.recvSocket)
		msg, err := n.recvSocket.RecvMessage()
		fmt.Println("msg: ", msg)
		if err != nil {
			fmt.Println("Error receiving message:", err)
			log.Fatal(err)
		}
		fmt.Println("Received", msg)

		// TODO: handle message
	}
}

func getPeersFromConfig(pathToConfig string) []string {
	// read json file
	jsonFile, err := os.Open(pathToConfig)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()

	// parse json file
	var addresses []string
	decoder := json.NewDecoder(jsonFile)
	err = decoder.Decode(&addresses)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return []string{}
	}

	return addresses
}

// startElectionTimer starts the election timer with a random timeout between 150ms and 300ms.
func (n *RaftNode) startElectionTimer() {
	timeout := time.Duration(rand.Intn(150)+150) * time.Millisecond
	n.electionTimer = time.NewTimer(timeout)
}

// becomeCandidate transitions the node to the candidate state and starts a new election.
func (n *RaftNode) becomeCandidate() {
	fmt.Println("Becoming candidate")
	n.state = "candidate"
	n.currentTerm++
	n.votedFor = n.name
	n.startElectionTimer()
}

// requestVote sends a vote request to the given node and returns true if the vote is granted.
func (n *RaftNode) requestVote() {
	fmt.Println("Requesting vote")
	// Increment the current term and vote for ourselves.
	n.currentTerm++
	n.votedFor = n.name

	// Create the request message.
	request := Message{
		Type:    "RequestVote",
		Term:    n.currentTerm,
		From:    n.name,
		To:      "",
		Command: "",
	}

	// Encode the request message as JSON.
	payload, err := json.Marshal(request)
	if err != nil {
		fmt.Println("Error encoding message:", err)
		return
	}

	// Send the request message to all other nodes.
	// for _, peer := range n.peers {
	// 	// serialize the message
	// 	err := n.dealerSocket.SendFrame([]byte(payload), goczmq.FlagNone)
	// 	if err != nil {
	// 		fmt.Println("Error sending message:", err)
	// 		return
	// 	}
	// }

	fmt.Println("Sending message")
	fmt.Println("payload: ", payload)
	err = n.dealerSocket.SendFrame([]byte(payload), goczmq.FlagNone)
	if err != nil {
		fmt.Println("Error sending message:", err)
		return
	}
	log.Println("node: ", n.name, " sent message: ", request)
}

func (n *RaftNode) run() {
	fmt.Println("Running node: ", n.name)

	// Start the election timer.
	n.startElectionTimer()

	for {
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

func newRaftNode(name string, address string, peers []string) *RaftNode {
	return &RaftNode{
		name:    name,
		address: address,
		state:   "follower",
		peers:   peers,
	}
}

func main() {
	// parse command line arguments
	if len(os.Args) != 3 {
		fmt.Println("Usage <node name>")
		os.Exit(1)
	}
	name := os.Args[1]
	fmt.Println("name: ", name)

	// address is second argument
	address := os.Args[2]
	fmt.Println("address: ", address)

	// parse config from json
	peers := getPeersFromConfig("config.json")
	fmt.Println("peers: ", peers)

	// remove self from peers
	for i, peer := range peers {
		if peer == address {
			peers = append(peers[:i], peers[i+1:]...)
			break
		}
	}
	fmt.Println("peers: ", peers)

	node := newRaftNode(name, address, peers)
	fmt.Println("node: ", node.name)

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
	defer node.dealerSocket.Destroy()
	defer node.recvSocket.Destroy()
}
