package main

import (
	"github.com/kito0830/go-tcp-ip/network"
	"encoding/hex"
	"fmt"
)

func main() {
	network, _ := network.NewTun()
	network.Bind()

	for {
		pkt, _ := network.Read()
		fmt.Print(hex.Dump(pkt.Buf[:pkt.N]))
		network.Write(pkt)
	}
}
