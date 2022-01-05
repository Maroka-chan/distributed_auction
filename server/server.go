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
	"strconv"
)

type MutexV2Server struct {
	pb.UnimplementedCommunicationServiceServer
}

type Bid struct {
	bidderId int32
	bid      int32
}

var (
	highestBid  = make(chan Bid, 1)
	serfCluster *serf.Serf
	timestamp   int32 = 0
)

func main() {
	clusterAddr := os.Getenv("CLUSTER_ADDRESS")
	highestBid <- Bid{bid: 0, bidderId: -1}

	var serfErr error
	serfCluster, serfErr = InitSerfCluster(clusterAddr)
	if serfErr != nil {
		log.Fatal(serfErr)
	}
	defer serfCluster.Leave()

	StartServer()

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
	CalculateTimestamp(bid.Timestamp)
	currentHighestBid := <-highestBid
	if bid.Amount > currentHighestBid.bid {
		highestBid <- Bid{bid: bid.Amount, bidderId: bid.Id}
		timestamp++
		return &pb.Acknowledgement{Message: "Bid registered.", Timestamp: timestamp}, nil
	}

	highestBid <- currentHighestBid
	timestamp++
	return &pb.Acknowledgement{Message: "Bid lower than current highest bid.", Timestamp: timestamp}, nil
}

func (s *MutexV2Server) RequestState(ctx context.Context, ts *pb.Timestamp) (outcome *pb.Outcome, err error) {
	CalculateTimestamp(ts.Timestamp)
	currentHighestBid := <-highestBid
	highestBid <- currentHighestBid
	timestamp++
	return &pb.Outcome{Timestamp: timestamp, Message: "The highest bidder has the id " +
		strconv.Itoa(int(currentHighestBid.bidderId)) + " with a bid of " +
		strconv.Itoa(int(currentHighestBid.bid))}, err
}

func CalculateTimestamp(otherTimestamp int32) {
	if otherTimestamp > timestamp {
		timestamp = otherTimestamp + 1
	} else {
		timestamp += 1
	}
}
