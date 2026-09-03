package scanner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/scannerworker/gen"
	"github.com/navidrome/navidrome/log"
	"google.golang.org/grpc"
)

type progressGRPCServer struct {
	gen.UnimplementedScanEventsServer
	bus *eventbus.Bus
}

func (s *progressGRPCServer) ReportProgress(stream gen.ScanEvents_ReportProgressServer) error {
	for {
		evt, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&gen.ProgressAck{Ok: true})
		}
		if err != nil {
			return err
		}
		s.bus.PublishSync(stream.Context(), eventbus.Event{
			Topic:        eventbus.TopicScanProgress,
			ScanProgress: ProgressToEvent(progressFromProto(evt)),
		})
	}
}

type progressListener struct {
	server *grpc.Server
	addr   string
}

func startProgressListener(addr string) (*progressListener, error) {
	if addr == "" {
		addr = rustworker.DefaultListenAddr("navidrome-scan-progress")
	}
	lis, err := listenScanProgress(addr)
	if err != nil {
		return nil, err
	}
	srv := grpc.NewServer()
	gen.RegisterScanEventsServer(srv, &progressGRPCServer{bus: eventbus.Get()})
	go func() {
		if serveErr := srv.Serve(lis); serveErr != nil {
			log.Debug("Scan progress gRPC server stopped", serveErr)
		}
	}()
	return &progressListener{server: srv, addr: addr}, nil
}

func (l *progressListener) Stop() {
	if l == nil || l.server == nil {
		return
	}
	l.server.GracefulStop()
}

func listenScanProgress(addr string) (net.Listener, error) {
	if path, ok := strings.CutPrefix(addr, "unix:"); ok {
		rustworker.UnlinkUnixListen(addr)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("creating scan progress socket dir: %w", err)
		}
		return net.Listen("unix", path)
	}
	network := "tcp"
	host := addr
	if strings.HasPrefix(addr, "tcp://") {
		host = strings.TrimPrefix(addr, "tcp://")
	}
	return net.Listen(network, host)
}

func ReportProgressGRPC(ctx context.Context, addr string, progress <-chan *ProgressInfo) error {
	conn, err := rustworker.DialGRPC(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	cli := gen.NewScanEventsClient(conn)
	stream, err := cli.ReportProgress(ctx)
	if err != nil {
		return err
	}
	for info := range progress {
		if err := stream.Send(progressToProto(info)); err != nil {
			return err
		}
	}
	_, err = stream.CloseAndRecv()
	return err
}
