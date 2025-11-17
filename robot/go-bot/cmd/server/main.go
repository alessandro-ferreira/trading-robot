package main

import (
	"context"
	"log"
	"time"

	pb "trading/robot/go-bot/gen/go/v1" // Import the generated Go code

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	address = "localhost:50051" // The Python server address
)

func main() {
	// Set up a connection to the server.
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewExchangeServiceClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	log.Println("Sending CreateOrder request to Python server...")
	r, err := c.CreateOrder(ctx, &pb.CreateOrderRequest{Symbol: "BTC/BRL", Type: pb.OrderType_MARKET, Side: pb.OrderSide_BUY, Amount: 0.001})
	if err != nil {
		log.Fatalf("could not create order: %v", err)
	}
	log.Printf("✅ Success! Response from server: OrderID=%s, Status=%s", r.GetOrderId(), r.GetStatus())
}
