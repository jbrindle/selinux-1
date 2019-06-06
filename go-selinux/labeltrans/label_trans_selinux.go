// +build selinux,linux

package labeltrans

import (
	"fmt"
	"github.com/google/vectorio"
 	"log"
	"net"
	"syscall"
	"unsafe"
)

var sockpath = "/var/run/setrans/.setrans-unix"

type RequestType uint32

const (
	ReqRawToTrans RequestType = 2
	ReqTransToRaw RequestType = 3
	ReqRawToColor RequestType = 4
)

type request struct {
	label   string
	reqType RequestType
	res     chan<- string
}

var reqchan chan request

func sendRequest(c *net.UnixConn, t RequestType, data string) (resp string, Err error) {
	const num_iovecs = 5

	// mcstransd expects null terminated strings which go does not do, so we append nulls here
	d := data + "\000"
	data_size := uint32(len(d))

	data2 := "\000" // unused by libselinux users
	data2_size := uint32(len(data2))

	v := make([]syscall.Iovec, num_iovecs)

	d1 := []byte(d)
	d2 := []byte(data2)

	v[0] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&t)), Len: uint64(unsafe.Sizeof(t))}
	v[1] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&data_size)), Len: uint64(unsafe.Sizeof(data_size))}
	v[2] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&data2_size)), Len: uint64(unsafe.Sizeof(data2_size))}
	v[3] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&d1[0])), Len: uint64(data_size)}
	v[4] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&d2[0])), Len: uint64(data2_size)}

	f, _ := c.File()
	written, err := vectorio.WritevRaw(f.Fd(), v)

	fmt.Printf("sent %d bytes, err: %e\n", written, err)

	//buff := make([]byte, 1024)
	//oob := make([]byte, 1024)

	/*
	   _,_,_,_,err = c.ReadMsgUnix(buff,oob);
	   if err != nil {
	       fmt.Println(err)
	   }
	*/

	hdr := make([]syscall.Iovec, 3)

	var elem uint32
	elemsize := uint64(unsafe.Sizeof(elem)) 

	hdr[0].Len = elemsize   // function  
	hdr[1].Len = elemsize   // response length
	hdr[2].Len = elemsize   // return value

	len, err := vectorio.ReadvRaw(f.Fd(), hdr)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("Function: %d size: %d ret: %d and bytes recv: %d\n", *hdr[0].Base, *hdr[1].Base, *hdr[2].Base, len)

	respvec := make([]syscall.Iovec, 1)
	respvec[0].Len = uint64(*hdr[1].Base)

	len, err = vectorio.ReadvRaw(f.Fd(), respvec)
	if err != nil {
		fmt.Println(err)
	}

	b := *(*[]byte)(unsafe.Pointer(&respvec[0].Base))
	resp = string(b[:len - 1]) // mcstransd adds a null to the end, remove it

	fmt.Printf("Response: %q and bytes recv: %d\n", resp, len)

	return resp, nil
}

func connect() {
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: sockpath, Net: "unix"})
	if err != nil {
		log.Fatal("Dial error", err)
	}
	defer c.Close()

	fmt.Println("Ready to recieve requests")
	for {
		select {
		case req := <-reqchan:
			fmt.Printf("Got request %s\n", req.label)
			res, _ := sendRequest(c, ReqRawToTrans, req.label)
			req.res <- res
		}
	}
}

func TransToRaw(trans string) (raw string, Err error) {
	if reqchan == nil {
		reqchan = make(chan request)
		go connect()
	}

	fmt.Printf("Ready to request translation of %s \n", trans)
	reschan := make(chan string)
	translated := request{trans, ReqTransToRaw, reschan}
	fmt.Println("Sending request")
	reqchan <- translated
	fmt.Println("Waiting for response")
	raw = <-reschan
	fmt.Printf("Got back %s\n", raw)
	return
}

func RawToTrans(trans string) (raw string, Err error) {
	if reqchan == nil {
		reqchan = make(chan request)
		go connect()
	}

	fmt.Printf("Ready to request translation of %s \n", trans)
	reschan := make(chan string)
	translated := request{trans, ReqRawToTrans, reschan}
	fmt.Println("Sending request")
	reqchan <- translated
	fmt.Println("Waiting for response")
	raw = <-reschan
	fmt.Printf("Got back %s\n", raw)
	return
}
