# [WIP] Go Raft

## Requirements

1. Install [libzmq](https://zeromq.org/download/)
2. Install [CZMQ](https://zeromq.org/languages/c/)
3. Install go run tidy command
    - `go mod tidy`

## How to

1. You can run a node once all the requirements are met

```bash
go run . <node-name> <node-address>
```