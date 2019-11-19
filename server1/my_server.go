package main

/*
   目前已经实现

   第一步，先实现graceful shutdown
   第二步，实现重启
*/
import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/XuSenqi/my_tcp_graceful_restart/tcpserver"
)

func main() {

	var err error
	srv := tcpserver.NewServer()
	if os.Getenv("_GRACEFUL_RESTART") == "true" {
		fmt.Println("before NewFromFD")
		srv, err = tcpserver.NewFromFD(3, EchoHandler)
		if err != nil {
			fmt.Println("NewFromFD failed, err: ", err)
		}
		fmt.Println("afterNewFromFD")
	} else {
		srv = tcpserver.NewServer(
			tcpserver.Network("tcp"),
			tcpserver.Address("127.0.0.1:8080"),
			tcpserver.Handler(EchoHandler),
		)
	}

	processed := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func(cancel context.CancelFunc) {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP, syscall.SIGTERM)
		<-c

		/*
		   起来子进程
		*/
		listenerFD, err := srv.GetListenerFD()
		if err != nil {
			fmt.Println("srv.GetListenerFD err: ", err)
			close(processed)
			return
		}
		forkAndRun(listenerFD)
		// 问题：  但一直卡在这里，父进程退不出来 --》 改成ForkExec后解决

		fmt.Println("before stop()")
		srv.Stop()

		// 现在的问题： 子进程起来后，一直没有响应  --> 改成NewFroFD试试

		fmt.Println("before cancel()")
		cancel()
		fmt.Println("server gracefully shutdown")

		close(processed)
	}(cancel)

	fmt.Println("before Serve")
	go srv.Serve(ctx)

	<-processed

}

func forkAndRun(listenerFD uintptr) {
	// Set a flag for the new process start process
	os.Setenv("_GRACEFUL_RESTART", "true")

	execSpec := &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd(), listenerFD},
	}
	// Fork exec the new version of your server
	fork, err := syscall.ForkExec(os.Args[0], os.Args, execSpec)
	if err != nil {
		fmt.Println("Fail to fork", err)
	}
	fmt.Println("SIGHUP received: fork-exec to", fork)
}

// 起子进程后，会一直卡着，导致父进程退不出来
//func forkAndRun(l *net.TCPListener) {
//	//l := ln.(*net.TCPListener)
//	newFile, _ := l.File()
//	fmt.Println(newFile.Fd())
//
//	cmd := exec.Command(os.Args[0])
//	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
//	cmd.ExtraFiles = []*os.File{newFile}
//	cmd.Run()
//}

func EchoHandler(con net.Conn) error {
	defer con.Close()

	fmt.Println("[Server] Sent 'server-data'\n")
	_, err := con.Write([]byte("server-data"))
	if err != nil {
		fmt.Println("[Error] fail to write 'server-data':", err)
		con.Close()
		return err
	}

	return nil
}
