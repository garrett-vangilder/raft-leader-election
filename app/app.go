package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"context"

	"github.com/redis/go-redis/v9"
)

func getLeaderFromEndpoint() string {
	url := "http://localhost:7777"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return ""
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return ""
	}

	fmt.Println(string(body)) // Output the response body as a string
	// return string(body)
	return "true"
}

var ctx = context.Background()

func writeToCache() {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Replace with the address of your Redis server
		Password: "",               // Replace with the password of your Redis server, if applicable
		DB:       0,                // Replace with the desired database number
	})

	err := client.Set(ctx, "mykey", "myvalue", 0).Err()
	if err != nil {
		fmt.Println("Error writing to Redis:", err)
		return

	}
	fmt.Println("Value written successfully!")

	val, err := client.Get(ctx, "mykey").Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("key", val)
}

func main() {
	// call endpoint localhost:7777 (sidecar in same pod ) to get leader
	// leader := os.Getenv("RAFT_LEADER")
	isLeader := getLeaderFromEndpoint()
	fmt.Printf("Am I the leader? %s\n", isLeader)

	// // write to cache if this is the leader
	if isLeader == "true" {
		writeToCache()
	}
}
