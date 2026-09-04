package integration

import (
	"context"

	"github.com/navidrome/navidrome/log"
)

const integrationWorkerRestartLimit = 3

func (g *Gateway) attachWorker(client *grpcClient) {
	if client == nil {
		return
	}
	client.onDead = g.scheduleWorkerRestart
}

func (g *Gateway) scheduleWorkerRestart() {
	go g.tryRestartWorker(nil)
}

func (g *Gateway) tryRestartWorker(cause error) {
	g.grpcMu.Lock()
	defer g.grpcMu.Unlock()
	if g.shutdown || !g.workerExpected {
		return
	}
	if g.restarts >= integrationWorkerRestartLimit {
		log.Error("Integration worker restart limit reached", cause)
		if g.grpc != nil {
			g.grpc.close()
			g.grpc = nil
		}
		return
	}
	if g.grpc != nil {
		g.grpc.close()
		g.grpc = nil
	}
	client, err := startGRPCClient(context.Background())
	if err != nil {
		log.Error("Integration worker restart failed", cause, err)
		return
	}
	g.restarts++
	g.grpc = client
	g.attachWorker(client)
	log.Info("Integration gRPC worker restarted", "attempt", g.restarts, "cause", cause)
}
