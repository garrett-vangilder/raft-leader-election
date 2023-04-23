package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
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

	mutex   sync.Mutex
	reqSock *zmq.Socket
	repSock *zmq.Socket
	poller  *zmq.Poller
	context *zmq.Context

	// reference to peer addresses
	peers []Peer

	// reference to votes received
	votesReceived int
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

	// create a context
	context, err := zmq.NewContext()
	if err != nil {
		log.Fatal(err)
	}

	n.context = context

	// Create a router socket and bind it to address.
	fmt.Println("raft::connectToPeers() - creating router socket")

	rep, err := zmq.NewSocket(zmq.ROUTER)
	rep.Bind("tcp://*:" + n.port)

	if err != nil {
		fmt.Println("raft::connectToPeers() - failed to create new socket: ", err)
	}

	// create poller
	fmt.Println("raft::connectToPeers() - creating poller")
	poller := zmq.NewPoller()
	if err != nil {
		log.Fatal(err)
	}

	poller.Add(rep, zmq.POLLIN)

	// // create socket
	fmt.Println("raft::connectToPeers() - creating dealer socket")
	req, err := zmq.NewSocket(zmq.DEALER)
	if err != nil {
		log.Fatal(err)
	}

	// // connect to peers
	for _, peer := range n.peers {
		fmt.Println("raft::connectToPeers() - Connecting to peer:", peer.name+" at "+peer.address)
		// todo verify
		req.Connect("tcp://" + peer.address)
	}

	// add dealer to poller
	poller.Add(req, zmq.POLLIN)

	n.repSock = rep
	n.reqSock = req
	n.poller = poller
}

func (n *RaftNode) listenForResponses() {
	fmt.Println("raft::listenForResponses()")
	for {
		// declare message
		var msg []string

		// read from poller
		sockets, _ := n.poller.Poll(-1)

		for _, socket := range sockets {
			switch s := socket.Socket; s {
			case n.reqSock:
				fmt.Println("Received message from dealerSocket")
				_msg, err := s.RecvMessage(0)
				if err != nil {
					fmt.Println("Error receiving message:", err)
					continue
				}
				msg = _msg
			case n.repSock:
				_msg, err := s.RecvMessage(0)
				if err != nil {
					fmt.Println("Error receiving message:", err)
					continue
				}
				msg = _msg
			}

			fmt.Println("raft::listenForResponses() - message: ", msg)

			// decode message
			var message Message
			fmt.Println(message)
			err := json.Unmarshal([]byte(msg[1]), &message)
			if err != nil {
				fmt.Println("Error decoding message:", err)
				continue
			}

			// drop the message if its from us
			if message.To == n.name {
				fmt.Println("raft::listenForResponses() - message is for this node")

				// print the message
				fmt.Println("raft::listenForResponses() - message type: ", message.Type)
				fmt.Println("raft::listenForResponses() - message term: ", message.Term)
				fmt.Println("raft::listenForResponses() - message from: ", message.From)
				fmt.Println("raft::listenForResponses() - message to: ", message.To)
				n.receiveMessage(message)
			}
		}
	}
}

func (n *RaftNode) receiveMessage(message Message) {
	fmt.Println("raft::receiveMessage()")
	fmt.Println("raft::receiveMessage() - message type: ", message.Type)
	fmt.Println("raft::receiveMessage() - current state: ", n.state)

	switch n.state {
	case "follower":
		if message.Type == "RequestVote" {
			fmt.Println("raft::receiveMessage() - message type is RequestVote")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// check if message term is greater than current term
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - current term: ", n.currentTerm)
			if message.Term > n.currentTerm {
				fmt.Println("raft::receiveMessage() - message term is greater than current term")
				fmt.Println("raft::receiveMessage() - becoming follower")
				n.becomeFollower(message.Term)

				// check if we have already voted for someone else
				if n.votedFor != "" {
					fmt.Println("raft::receiveMessage() - already voted for someone else")
					n.sendVoteResponse(message.From, false)
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

			// reset election timer
			n.startElectionTimer()

			// send response
			n.sendHeartBeatResponse(message.From)

		}
	case "candidate":
		fmt.Println("raft::receiveMessage() - candidate - got heeeeeere")
		// If the message is a vote request, grant or deny the vote.
		if message.Type == "RequestVoteResponse" {
			fmt.Println("raft::receiveMessage() - message type is RequestVoteResponse")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)
			fmt.Println("raft::receiveMessage() - message votegranted: ", message.VoteGranted)

			// check if vote was granted
			if message.VoteGranted {
				fmt.Println("raft::receiveMessage() - vote was granted")
				n.votesReceived++
				fmt.Println("raft::receiveMessage() - votes received: ", n.votesReceived)

				// check if we have received a majority of votes
				if n.votesReceived > len(n.peers)/2 {
					fmt.Println("raft::receiveMessage() - received majority of votes")
					n.becomeLeader()
				}

			} else {
				fmt.Println("raft::receiveMessage() - vote was not granted")
			}

		} else if message.Type == "HeartBeat" {
			fmt.Println("raft::receiveMessage() - message type is Heartbeat")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// print currnet term
			fmt.Println("raft::receiveMessage() - current term: ", n.currentTerm)

			// check if message term is greater than current term
			n.becomeFollower(message.Term)

			// reset election timer
			n.startElectionTimer()

			// send response
			n.sendHeartBeatResponse(message.From)

		} else if message.Type == "RequestVote" {
			fmt.Println("raft::receiveMessage() - message type is RequestVote")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// check if message term is greater than current term
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - current term: ", n.currentTerm)
			if message.Term > n.currentTerm {
				fmt.Println("raft::receiveMessage() - message term is greater than current term")
				fmt.Println("raft::receiveMessage() - becoming follower")
				n.becomeFollower(message.Term)

				// grant vote
				fmt.Println("raft::receiveMessage() - granting vote")
				n.votedFor = message.From
				n.sendVoteResponse(message.From, true)
			} else {
				// deny vote
				fmt.Println("raft::receiveMessage() - denying vote")
				n.sendVoteResponse(message.From, false)
			}
		}
	case "leader":
		// If the message is a vote request, grant or deny the vote.
		// If the message is a heartbeat from the leader with a higher term, become a follower.

		if message.Type == "RequestVoteResponse" {
			fmt.Println("raft::receiveMessage() - message type is RequestVoteResponse")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)
			fmt.Println("raft::receiveMessage() - message votegranted: ", message.VoteGranted)

			// check if vote was granted
			if message.VoteGranted {
				fmt.Println("raft::receiveMessage() - vote was granted")
				n.votesReceived++
				fmt.Println("raft::receiveMessage() - votes received: ", n.votesReceived)

				// check if we have received a majority of votes
				if n.votesReceived > len(n.peers)/2 {
					fmt.Println("raft::receiveMessage() - received majority of votes")
					n.becomeLeader()
				}

			} else {
				fmt.Println("raft::receiveMessage() - vote was not granted")
			}

		} else if message.Type == "HeartBeat" {
			fmt.Println("raft::receiveMessage() - message type is Heartbeat")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// print currnet term
			fmt.Println("raft::receiveMessage() - current term: ", n.currentTerm)

			// check if message term is greater than current term
			n.becomeFollower(message.Term)

			// reset election timer
			n.startElectionTimer()

			// send response
			n.sendHeartBeatResponse(message.From)

		} else if message.Type == "RequestVote" {
			fmt.Println("raft::receiveMessage() - message type is RequestVote")
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - message from: ", message.From)
			fmt.Println("raft::receiveMessage() - message to: ", message.To)
			fmt.Println("raft::receiveMessage() - message command: ", message.Command)

			// check if message term is greater than current term
			fmt.Println("raft::receiveMessage() - message term: ", message.Term)
			fmt.Println("raft::receiveMessage() - current term: ", n.currentTerm)
			if message.Term > n.currentTerm {
				fmt.Println("raft::receiveMessage() - message term is greater than current term")
				fmt.Println("raft::receiveMessage() - becoming follower")
				n.becomeFollower(message.Term)

				// grant vote
				fmt.Println("raft::receiveMessage() - granting vote")
				n.votedFor = message.From
				n.sendVoteResponse(message.From, true)
			} else {
				// deny vote
				fmt.Println("raft::receiveMessage() - denying vote")
				n.sendVoteResponse(message.From, false)
			}
		}
	}
}

func getPeersFromConfig(pathToConfig string, nodeName string) []Peer {
	fmt.Println("raft::getPeersFromConfig()")

	fmt.Println("raft::getPeersFromConfig() - pathToConfig: ", pathToConfig)
	fmt.Println("raft::getPeersFromConfig() - reading config file")
	rawData, err := ioutil.ReadFile(pathToConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Convert the byte slice to a string
	jsonString := string(rawData)

	var data []map[string]interface{}
	err = json.Unmarshal([]byte(jsonString), &data)
	if err != nil {
		fmt.Println("Error:", err)
		return nil
	}

	var peers []Peer

	for _, node := range data {
		// do not connect to ourself
		if node["name"] == nodeName {
			continue
		}

		fmt.Println("raft::getPeersFromConfig() - populating node: ", node["name"])
		peers = append(peers, Peer{
			name:    node["name"].(string),
			address: node["address"].(string),
		})

	}

	return peers
}

// startElectionTimer starts the election timer with a random timeout between 150ms and 300ms.
func (n *RaftNode) startElectionTimer() {
	fmt.Println("Starting election timer")
	timeout := time.Duration(rand.Intn(150)+150) * time.Millisecond

	fmt.Println("raft::startElectionTimer() - timeout: ", timeout)
	n.electionTimer = time.NewTimer(timeout)
}

// becomeCandidate transitions the node to the candidate state and starts a new election.
func (n *RaftNode) becomeCandidate() {
	// fmt.Println("Becoming candidate")
	n.state = "candidate"
	n.currentTerm++
	n.votedFor = n.name
	n.votesReceived = 1
	n.startElectionTimer()
}

func (n *RaftNode) becomeFollower(term int) {
	n.state = "follower"
	n.currentTerm = term
	n.votedFor = ""
	n.votesReceived = 0
	n.startElectionTimer()
}

func (n *RaftNode) becomeLeader() {
	fmt.Println("raft::becomeLeader()")
	n.state = "leader"
	n.votedFor = ""
	n.votesReceived = 0
	n.startElectionTimer()
	n.currentTerm++
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
		_, err = n.reqSock.SendBytes(payload, 0)

		if err != nil {
			fmt.Println("Error sending message:", err)
			return
		}
		fmt.Println("Message sent")

	}
}

// requestHeartBeat sends a heartbeat to the given node.
func (n *RaftNode) requestHeartBeat() {
	fmt.Println("raft::requestHeartBeat()")
	fmt.Println("raft::requestHeartBeat() - sending heartbeat to each peer")
	// send to each peer
	for _, peer := range n.peers {
		fmt.Println("raft::requestHeartBeat() - sending heartbeat to: ", peer)
		request := Message{
			Type:    "HeartBeat",
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
		_, err = n.reqSock.SendBytes(payload, 0)

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

	fmt.Println("raft::sendVoteResponse() - Sending message")
	_, err = n.reqSock.SendBytes(payload, 0)
	if err != nil {
		fmt.Println("Error sending message:", err)
		return
	}

	fmt.Println("Message sent")
}

// sendHeartBeatResponse sends a heartbeat response to the given node.
func (n *RaftNode) sendHeartBeatResponse(to string) {
	fmt.Println("raft::sendHeartBeatResponse()")
	fmt.Println("raft::sendHeartBeatResponse() - to: ", to)
	// Create the response message.
	response := Message{
		Type:        "HeartBeatResponse",
		Term:        n.currentTerm,
		From:        n.name,
		To:          to,
		Command:     "",
		VoteGranted: false,
	}

	// Encode the response message as JSON.
	payload, err := json.Marshal(response)
	if err != nil {
		fmt.Println("Error encoding message:", err)
		return
	}

	fmt.Println("raft::sendHeartBeatResponse() - Sending message")
	_, err = n.repSock.SendBytes(payload, 0)
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

			// If a leader is elected, change state and return.
			// If the election timer expires, start a new election.
			select {
			case <-n.electionTimer.C:
				n.becomeCandidate()
			}
			// If a vote request or heartbeat message is received from the leader, become a follower.
		case "leader":
			// Send heartbeat messages to all other nodes.
			n.requestHeartBeat()

			// If a node does not receive a heartbeat within the election timeout, start a new election.
			// If a node receives a higher term from a message, become a follower.
		}
	}
}

func newRaftNode(name string, address string, port string, peers []Peer) *RaftNode {
	fmt.Println("raft::newRaftNode()")

	return &RaftNode{
		name:    name,
		address: address,
		port:    port,
		state:   "follower",
		peers:   peers,
	}
}

func webHandler(res http.ResponseWriter, req *http.Request) {
	// randomize for testing
	rand.Seed(time.Now().UnixNano())
	isLeader := rand.Intn(2) == 1

	data, err := json.Marshal(isLeader)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}

func main() {

	fmt.Println("raft::main")

	// parse command line arguments
	if len(os.Args) != 4 {
		fmt.Println("Usage <node name> <address> <port>")
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
	peers := getPeersFromConfig("/etc/raftconfig/config.json", name)
	//peers := getPeersFromConfig("./config.json", name)

	// remove self from peers
	for i, peer := range peers {
		if peer.address == address+":"+port {
			peers = append(peers[:i], peers[i+1:]...)
			break
		}
	}

	node := newRaftNode(name, address, port, peers)

	// connect node to peers
	node.connectToPeers()
	// Start listening for responses on the ZeroMQ socket.
	go node.listenForResponses()

	// Start the process
	go node.run()

	fmt.Println("raft::main - http listener")
	http.HandleFunc("/", webHandler)
	http.ListenAndServe("localhost:7777", nil)

	// infinite loop to prevent exit
	for {
		time.Sleep(1 * time.Second)
	}

	// cleanup sockets
	defer node.reqSock.Close()
	defer node.repSock.Close()
	defer node.context.Term()
}
