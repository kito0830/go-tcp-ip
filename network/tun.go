package network

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"unsafe"
	"log"
)

const (
	TUNSETIFF   = 0x400454ca // TUNインターフェースの設定を行うためのioctl（input/output control）システムコールで使用される定数
	IFF_TUN     = 0x0001     // TUNインターフェースを指定するためのフラグ
	IFF_NO_PI   = 0x1000     // パケットの先頭にプロトコルヘッダーを付加しないためのフラグ
	PACKET_SIZE = 2048       // パケットのサイズ
	QUEUE_SIZE  = 10         // キューのサイズ
)

type Packet struct {
	Buf []byte
	N   uintptr
}

type ifreq struct {
	ifrName  [16]byte
	ifrFlags int16
}

type NetDevice struct {
	file          *os.File
	incomingQueue chan Packet
	outgoingQueue chan Packet
	ctx           context.Context
	cancel        context.CancelFunc
}

// TUNを作成する
func NewTun() (*NetDevice, error) {
	/*
		os.OpenFile：ファイルを開く関数
		第一引数：ファイル名
		第二引数：ファイルのオープンモード(読み書き可能な状態で開く)
		第三引数：0を指定するとファイルの新規作成を許可する
	*/
	file, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open error: %s", err.Error())
	}

	/* ネットワークインターフェースの設定
	（ネットワーク層、データリンク層の1つ上の話、レイヤー3）
	*/
	ifr := ifreq{}                       // ifreq構造体の初期化
	copy(ifr.ifrName[:], []byte("tun0")) // 文字列をバイト列に変換してスライスに格納、ifreq.ifrNameにコピーしている。スライスの中には10新数が入っている
	ifr.ifrFlags = IFF_TUN | IFF_NO_PI   // IFF_TUNとIFF_NO_PIをOR演算してifreq.ifrFlagsに格納

	/* システムコールを呼び出す（システムコール：OSの機能を呼び出すための仕組み）
	tun0デバイスに対して、ifreq構造体の情報を設定するためのioctlシステムコールを呼び出している
	*/
	_, _, sysErr := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(TUNSETIFF), uintptr(unsafe.Pointer(&ifr)))
	if sysErr != 0 {
		return nil, fmt.Errorf("ioctl error: %s", sysErr.Error())
	}

	return &NetDevice{
		file:          file,
		incomingQueue: make(chan Packet, QUEUE_SIZE),
		outgoingQueue: make(chan Packet, QUEUE_SIZE),
	}, nil
}


// TUN デバイスに対して、syscall.SYS_READとsyscall.SYS_WRITEを使用してパケットの送受信を行う
func (t *NetDevice) read(buf []byte) (uintptr, error) {
	n, _, sysErr := syscall.Syscall(syscall.SYS_READ, t.file.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if sysErr != 0 {
		return 0, fmt.Errorf("read error: %s", sysErr.Error())
	}
	return n, nil
}

func (t *NetDevice) write(buf []byte) (uintptr, error) {
	n, _, sysErr := syscall.Syscall(syscall.SYS_WRITE, t.file.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if sysErr != 0 {
		return 0, fmt.Errorf("write error: %s", sysErr.Error())
	}
	return n, nil
}

func (tun *NetDevice) Bind() {
	tun.ctx, tun.cancel = context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case <-tun.ctx.Done():
				return
			default:
				buf := make([]byte, PACKET_SIZE)
				n, err := tun.read(buf)
				if err != nil {
					log.Printf("read error: %s", err.Error())
				}
				packet := Packet{
					Buf: buf[:n],
					N:   n,
				}
				tun.incomingQueue <- packet
			}
		}
	}()

	go func() {
		for {
			select {
			case <-tun.ctx.Done():
				return
			case pkt := <-tun.outgoingQueue:
				_, err := tun.write(pkt.Buf[:pkt.N])
				if err != nil {
					log.Printf("write error: %s", err.Error())
				}
			}
		}
	}()
}