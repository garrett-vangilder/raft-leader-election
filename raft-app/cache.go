package main

import (
	"context"
	"fmt"

	"github.com/go-redis/redis"
)

var ctx = context.Background()

func writeToCache(key, value string) bool {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Replace with the address of your Redis server
		Password: "irTC1asfPA",     // Replace with the password of your Redis server, if applicable
		DB:       0,                // Replace with the desired database number
	})

	err := client.Set(key, value, 0).Err()
	if err != nil {
		fmt.Println("Error writing to Redis:", err)
		return false

	}
	fmt.Println("Value written successfully!")

	val, err := client.Get(key).Result()
	if err != nil {
		panic(err)
	}
	fmt.Println(key, val)
	return true
}
