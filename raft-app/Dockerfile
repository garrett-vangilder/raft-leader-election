FROM golang:1.20

RUN apt-get update -qq && \
    apt-get install -y -qq libzmq3-dev 

WORKDIR /app
COPY go.mod . go.sum ./
RUN go mod download
COPY *.go ./

RUN go build -o raft-node

EXPOSE 8080

#CMD ["/app/raft-node"]
