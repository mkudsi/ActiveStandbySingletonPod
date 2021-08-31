package main
import (
    "net"
    "fmt"
    "time"
)


func main() {

  fmt.Println("Started UDP server. Waiting for message from controller curr-time : ", time.Now(), "\n\n")
  ServerConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP:[]byte{0,0,0,0},Port:10001,Zone:""})
  defer ServerConn.Close()
  buf := make([]byte, 1024)
  for {
    n, addr, _ := ServerConn.ReadFromUDP(buf)
    fmt.Println("Received ", string(buf[0:n]), " from ", addr, " at ", time.Now())
  }
}

