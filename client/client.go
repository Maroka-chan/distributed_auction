package main

import (
	"bufio"
	"context"
	pb "example.com/MutexV2/service"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	timestamp int32 = 0
	id        int32
)

func main() {
	id = int32(uuid.New().ID())

	var conn, conErr = Connect()
	if conErr != nil {
		log.Fatalf("could not connect to a server: %v", conErr)
	}

	defer conn.Close()
	var client = pb.NewCommunicationServiceClient(conn)
	var ctx = context.Background()

	go ListenForUserInput(ctx, client)

	select {}
}

func Connect() (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	var connErr error
	conn, connErr = grpc.Dial(":8080", grpc.WithInsecure(), grpc.WithBlock())
	if connErr != nil {
		log.Printf("did not connect: %v", connErr)
	}
	return conn, connErr
}

func ListenForUserInput(ctx context.Context, client pb.CommunicationServiceClient) {
	var in string
	sc := bufio.NewScanner(os.Stdin)
	for {
		if sc.Scan() {
			in = sc.Text()
			args := strings.Split(in, " ")
			switch args[0] {
			case "bid":
				amount, convErr := strconv.Atoi(args[1])
				if convErr != nil {
					log.Printf("Insert a valid 32bit integer value. %v", convErr)
					break
				}
				timestamp++
				bidRes, bidErr := client.RequestBid(ctx, &pb.Bid{Id: id, Amount: int32(amount), Timestamp: timestamp})
				if bidErr != nil {
					log.Printf("Failed to bid: %v", bidErr)
					break
				}
				CalculateTimestamp(bidRes.Timestamp)
				log.Printf(bidRes.Message)
				break
			case "state":
				timestamp++
				bidRes, bidErr := client.RequestState(ctx, &pb.Timestamp{Timestamp: timestamp})
				if bidErr != nil {
					log.Printf("Failed to bid: %v", bidErr)
					break
				}
				CalculateTimestamp(bidRes.Timestamp)
				log.Printf(bidRes.Message)
				break
			}
		}
	}
}

func CalculateTimestamp(otherTimestamp int32) {
	if otherTimestamp > timestamp {
		timestamp = otherTimestamp + 1
	} else {
		timestamp += 1
	}
}
