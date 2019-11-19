package tcpserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

//ConnectionHandler connection handler definition
type ConnectionHandler func(conn net.Conn) error

type Server struct {
	network  string
	address  string
	handler  ConnectionHandler
	listener *net.TCPListener
	wgroup   sync.WaitGroup
}

func (srv *Server) GetListenerFD() (uintptr, error) {
	file, err := srv.listener.File()
	if err != nil {
		return 0, err
	}
	return file.Fd(), nil
}

//ServerOpt typedef
type ServerOpt func(*Server)

//Network option for listener
func Network(inet string) ServerOpt {
	return func(srv *Server) {
		if len(inet) != 0 {
			srv.network = inet
		}
	}
}

//Address option for listener
func Address(addr string) ServerOpt {
	return func(srv *Server) {
		if len(addr) != 0 {
			srv.address = addr
		}
	}
}

//Handler option for connection
func Handler(h ConnectionHandler) ServerOpt {
	return func(srv *Server) {
		if h != nil {
			srv.handler = h
		}
	}
}

func Listener(listener *net.TCPListener) ServerOpt {
	return func(srv *Server) {
		if listener != nil {
			srv.listener = listener
		}
	}
}

//NewServer create a new tcpserver
func NewServer(opts ...ServerOpt) *Server {
	serv := &Server{}
	for _, opt := range opts {
		opt(serv)
	}
	return serv
}

func NewFromFD(fd uintptr, h ConnectionHandler) (*Server, error) {

	file := os.NewFile(fd, "graceful-restart")
	listener, err := net.FileListener(file)
	if err != nil {
		return nil, errors.New("File to recover socket from file descriptor: " + err.Error())
	}
	listenerTCP, ok := listener.(*net.TCPListener)
	if !ok {
		return nil, fmt.Errorf("File descriptor %d is not a valid TCP socket", fd)
	}
	srv := &Server{listener: listenerTCP, handler: h}

	return srv, nil
}

func (srv *Server) Serve(ctx context.Context) error {
	if srv.listener == nil {
		ln, err := net.Listen(srv.network, srv.address)
		if err != nil {
			return err
		}
		fmt.Println("tcpserver serving at ", srv.network, srv.address)
		srv.listener = ln.(*net.TCPListener)
	}
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Serve: ctx.Done()")
			return ctx.Err()
		default:
			fmt.Println("before accept:")
			con, err := srv.listener.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					fmt.Printf("warning: accept temp err: %v", ne)
					continue
				}
				fmt.Println("accept error:", err)
				return err
			}

			srv.wgroup.Add(1)
			go func() {
				defer srv.wgroup.Done()
				if err := srv.handler(con); err != nil {
					log.Printf("connection %s handle failed: %s\n", con.RemoteAddr().String(), err)
				}
			}()
		}
	}
}

// Accept will instantly return a timeout error
func (srv *Server) Stop() {
	srv.listener.Close()
	srv.wgroup.Wait()
}
