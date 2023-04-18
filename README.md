# [WIP] Go Raft

## Requirements

1. Install [libzmq](https://zeromq.org/download/)
2. Install [CZMQ](https://zeromq.org/languages/c/)
3. Install go run tidy command
    - `go mod tidy`

## How to

1. You can run a node once all the requirements are met

```bash
go run . <node-name> <node-address> <node-port>
```


## Docker

You can run nodes 1, 2 & 3 together in containers using the docker-compose file, by running:

```
docker compose up
```

## Kubernetes

The follow steps will get the raft nodes deployed in Kubernetes locally. Go to a command line, ensure you have `kubectl` installed, and do the following:

Create a namespace to run them in:
```
kubectl create namespace raft
```

Set the default namespace to that:
```
kubectl config set-context --current --namespace=raft
```

To deploy the nodes into Kubernetes, run the following when pointed to a k8s cluster. It will create the pods via a Deployment, and expose them via a Service:
```
docker tag raft-node localhost:5001/raft-node:1.0 # assumes you ran docker-compose above and built this image
docker push localhost:5001/raft-node:1.0 

kubectl apply -f raft-node-deployment.yaml
kubectl apply -f raft-node-service.yaml
```

You can delete these with:

```
kubectl delete deployment raft-node
kubectl delete service raft-node
```

Demo: 
- Operator will launch raft nodes
- Show logs of election, heartbeats, statuses from follower to candidate, etc.
- Raft code calling k8s api to tag leader? 
- Leader writes to cache, show leader is writing
- Simulate a crash by deleting the leader and see new one elected 
