# Redis 

## Install Redis on your cluster using Helm

Update your Helm repo with Bitnami for the Redis Helm chart:
```
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update
```

Install Redis via the chart:
```
helm install redis-test --set persistence.storageClass=nfs-client,redis.replicas.persistence.storageClass=nfs-client bitnami/redis --set volumePermissions.enabled=true
```


## Connecting to Redis

Redis&reg; can be accessed on the following DNS names from within your cluster:

    redis-test-master.raft.svc.cluster.local for read/write operations (port 6379)
    redis-test-replicas.raft.svc.cluster.local for read-only operations (port 6379)

To get your password run:

    export REDIS_PASSWORD=$(kubectl get secret --namespace raft redis-test -o jsonpath="{.data.redis-password}" | base64 -d)

To connect to your Redis&reg; server:

1. Run a Redis&reg; pod that you can use as a client:

   kubectl run --namespace raft redis-client --restart='Never'  --env REDIS_PASSWORD=$REDIS_PASSWORD  --image docker.io/bitnami/redis:7.0.11-debian-11-r0 --command -- sleep infinity

   Use the following command to attach to the pod:

   kubectl exec --tty -i redis-client \
   --namespace raft -- bash

2. Connect using the Redis&reg; CLI:
   REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli -h redis-test-master
   REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli -h redis-test-replicas

To connect to your database from outside the cluster execute the following commands:

    kubectl port-forward --namespace raft svc/redis-test-master 6379:6379 &
    REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli -h 127.0.0.1 -p 6379