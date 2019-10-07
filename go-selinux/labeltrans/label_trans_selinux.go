// +build selinux,linux

package labeltrans

import (
	"fmt"
	"github.com/google/vectorio"
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

type reply struct {
	label string
	err error
}

type request struct {
	label   string
	reqType RequestType
	res     chan<- reply
}

var reqchan chan request
var active bool = true
var done bool = false

func sendRequest(c *net.UnixConn, t RequestType, data string) (resp string, Err error) {
	// mcstransd expects null terminated strings which go does not do, so we append nulls here
	d := data + "\000"
	data_size := uint32(len(d))

	data2 := "\000" // unused by libselinux users
	data2_size := uint32(len(data2))

	v := make([]syscall.Iovec, 5)

	d1 := []byte(d)
	d2 := []byte(data2)

	v[0] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&t)), Len: uint64(unsafe.Sizeof(t))}
	v[1] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&data_size)), Len: uint64(unsafe.Sizeof(data_size))}
	v[2] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&data2_size)), Len: uint64(unsafe.Sizeof(data2_size))}
	v[3] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&d1[0])), Len: uint64(data_size)}
	v[4] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(&d2[0])), Len: uint64(data2_size)}

	f, _ := c.File()
	defer f.Close()
	_, err := vectorio.WritevRaw(f.Fd(), v)
	if err != nil {
		// We are not going to receive anything if the write failed
		return "", err
	}

	hdr := make([]syscall.Iovec, 3)

	var elem uint32
	elemsize := uint64(unsafe.Sizeof(elem))

	hdr[0].Len = elemsize   // function
	hdr[1].Len = elemsize   // response length
	hdr[2].Len = elemsize   // return value

	len, err := vectorio.ReadvRaw(f.Fd(), hdr)
	if err != nil {
		// If the first read failed we will not know how long the response is
		return "", err
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

func connect() (c *net.UnixConn, err error) {
	c, err = net.DialUnix("unix", nil, &net.UnixAddr{Name: sockpath, Net: "unix"})
	if err != nil {
		// mcstrans is not running or we cannot connect for some reason, set the flag and return
		return nil, err
	}
	return
}

func manager() {
	c, _ := connect()
	if c == nil {
		done = true
		active = false
		return
	}
	defer c.Close()
	reqchan = make(chan request)
	done = true
	fmt.Println("Ready to recieve requests")
	for {
		select {
		case req := <-reqchan:
			fmt.Printf("Got request %s\n", req.label)
			res, err := sendRequest(c, req.reqType, req.label)
			if err != nil {
				fmt.Println(err)
				// let us try to reconnect once and try again before bailing
				c, err  = connect()
				defer c.Close()
				if err != nil {
					// still down		
					fmt.Println(err)
					active = false
					req.res <- reply{label:"", err:err}
					break
				}
				res, err = sendRequest(c, req.reqType, req.label)
				if err != nil {
					fmt.Println(err)
					active = false
					req.res <- reply{label:"", err:err}
				}
			}
			req.res <- reply{label:res, err:err}
		}
	}
}

func makeRequest(con string, t RequestType) (con2 string, Err error) {
	if reqchan == nil {
		go manager()
		// wait until we are connected to continue
		for
	}
	if active == false {
		fmt.Printf("returning %s due to inactive server\n", con)
		return con, nil
	}


	reschan := make(chan reply)
	translated := request{con, t, reschan}
	reqchan <- translated

	var resp reply
	resp = <-reschan

	con2 = resp.label
	Err = resp.err

	fmt.Printf("Got back %s\n", con2)
	return

}

func TransToRaw(trans string) (raw string, Err error) {
	raw, Err = makeRequest(trans, ReqTransToRaw)
	return
}

func RawToTrans(raw string) (trans string, Err error) {
	trans, Err = makeRequest(raw, ReqRawToTrans)
	return
}

func RawToColor(raw string) (color string, Err error) {
	color, Err = makeRequest(raw, ReqRawToColor)
	return
}

