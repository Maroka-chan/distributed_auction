FROM golang:1.17

ENV APP="server"
ENV CLUSTER_ADDRESS=""
ENV PATH="$PATH:$(go env GOPATH)/bin"

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...
RUN apt update
RUN apt install -y protobuf-compiler
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1

RUN /usr/bin/protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ./service/service.proto

ENTRYPOINT ["sh", "-c", "go run ./$APP"]