package main

import (
	"context"
	pb "example.com/MutexV2/service"
	"github.com/google/uuid"
	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"time"
)

var (
	serfCluster *serf.Serf
	highestBid  = make(chan Bid, 1)
	isOver      = false
)

type MutexV2Server struct {
	pb.UnimplementedCommunicationServiceServer
}

type Bid struct {
	bidderId int32
	bid      int32
}

func main() {
	clusterAddr := os.Getenv("CLUSTER_ADDRESS")
	highestBid <- Bid{bid: 0, bidderId: -1}

	timer := time.NewTimer(90 * time.Second)

	var serfErr error
	serfCluster, serfErr = InitSerfCluster(clusterAddr)
	if serfErr != nil {
		log.Fatal(serfErr)
	}
	defer serfCluster.Leave()

	go StartServer()

	<-timer.C // Waits for auction to conclude
	isOver = true

	select {}
}

func InitSerfCluster(clusterAddr string) (*serf.Serf, error) {
	conf := serf.DefaultConfig()
	conf.Init()
	conf.NodeName = uuid.New().String()

	cluster, serfErr := serf.Create(conf)
	if serfErr != nil {
		return nil, errors.Wrap(serfErr, "Couldn't create cluster")
	}

	if len(clusterAddr) > 0 {
		_, joinErr := cluster.Join([]string{clusterAddr}, true)
		if joinErr != nil {
			log.Printf("Couldn't join cluster: %v\n", joinErr)
		}
	}

	return cluster, nil
}

func StartServer() {
	server := grpc.NewServer()
	pb.RegisterCommunicationServiceServer(server, &MutexV2Server{})

	lis, servErr := net.Listen("tcp", ":8080")
	if servErr != nil {
		log.Fatalf("failed to listen: %v", servErr)
	}

	log.Printf("grpc server listening at %v", lis.Addr())
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *MutexV2Server) RequestBid(ctx context.Context, bid *pb.Bid) (acknowledgement *pb.Acknowledgement, err error) {
	currentHighestBid := <-highestBid
	if currentHighestBid.bid < bid.Amount {
		currentHighestBid = Bid{bidderId: bid.Id, bid: bid.Amount}
	}
	successful := false
	var clientsToRecover []pb.CommunicationServiceClient
	var highestTimestamp int32 = -1
	var highestTimestampClient pb.CommunicationServiceClient
	var acknowledgementToSend *pb.Acknowledgement
	for _, member := range serfCluster.Members() {
		if member.Addr.Equal(serfCluster.LocalMember().Addr) {
			continue
		}
		ctx, _ := context.WithTimeout(context.Background(), 1*time.Second)
		conn, connErr := grpc.DialContext(ctx, member.Addr.String()+":8080", grpc.WithInsecure(), grpc.WithBlock())
		if connErr != nil {
			continue
		}
		client := pb.NewCommunicationServiceClient(conn)
		if isOver {
			responseState, err := client.RequestState(ctx, &pb.Timestamp{Timestamp: bid.Timestamp})
			if err != nil {
				continue
			}
			return &pb.Acknowledgement{Message: "The auction has concluded and your bid have not been registered!\n" +
				"No more bids will be taken!", Timestamp: responseState.Timestamp}, nil
		}
		requestBid, err := client.RequestBid(ctx, bid)
		if err != nil {
			continue
		}
		successful = true
		if highestTimestamp == -1 {
			acknowledgementToSend = requestBid
			highestTimestamp = requestBid.Timestamp
			highestTimestampClient = client
		} else if highestTimestamp < requestBid.Timestamp {
			clientsToRecover = append(clientsToRecover, highestTimestampClient)
			highestTimestamp = requestBid.Timestamp
			highestTimestampClient = client
		} else if highestTimestamp > requestBid.Timestamp {
			clientsToRecover = append(clientsToRecover, client)
		}
	}
	RecoverClients(clientsToRecover, ctx, currentHighestBid, highestTimestamp)
	highestBid <- currentHighestBid

	if !successful {
		log.Fatalf("Could not connect to cluster!")
		return nil, err
	} else {
		return acknowledgementToSend, nil
	}
}

func (s *MutexV2Server) RequestState(ctx context.Context, ts *pb.Timestamp) (outcome *pb.Outcome, err error) {
	currentHighestBid := <-highestBid
	successful := false
	var clientsToRecover []pb.CommunicationServiceClient
	var highestTimestamp int32 = -1
	var highestTimestampClient pb.CommunicationServiceClient
	var outcomeToSend *pb.Outcome
	for _, member := range serfCluster.Members() {
		if member.Addr.Equal(serfCluster.LocalMember().Addr) {
			continue
		}
		ctx, _ := context.WithTimeout(context.Background(), 1*time.Second)
		conn, connErr := grpc.DialContext(ctx, member.Addr.String()+":8080", grpc.WithInsecure(), grpc.WithBlock())
		if connErr != nil {
			continue
		}
		client := pb.NewCommunicationServiceClient(conn)
		requestState, err := client.RequestState(ctx, ts)
		if err != nil {
			continue
		}
		successful = true
		if highestTimestamp == -1 {
			outcomeToSend = requestState
			highestTimestamp = requestState.Timestamp
			highestTimestampClient = client
		} else if highestTimestamp < requestState.Timestamp {
			clientsToRecover = append(clientsToRecover, highestTimestampClient)
			highestTimestamp = requestState.Timestamp
			highestTimestampClient = client
		} else if highestTimestamp > requestState.Timestamp {
			clientsToRecover = append(clientsToRecover, client)
		}
	}
	RecoverClients(clientsToRecover, ctx, currentHighestBid, highestTimestamp)
	highestBid <- currentHighestBid
	if !successful {
		log.Fatalf("Could not connect to cluster!")
		return nil, err
	} else {
		if isOver {
			return &pb.Outcome{Timestamp: outcomeToSend.Timestamp, Message: "The auction has concluded!\n" + outcomeToSend.Message}, nil
		}
		return outcomeToSend, nil
	}
}

func RecoverClients(clients []pb.CommunicationServiceClient, ctx context.Context, currentHighestBid Bid, highestTimestamp int32) {
	for _, client := range clients {
		requestBid, err := client.RequestBid(ctx, &pb.Bid{Amount: currentHighestBid.bid, Id: currentHighestBid.bidderId, Timestamp: highestTimestamp - 2})
		if err != nil {
			log.Fatalf("Something went wrong during recovery")
		}
		if requestBid.Timestamp != highestTimestamp {
			log.Fatalf("Timestamps are wrong")
		}
	}
}
