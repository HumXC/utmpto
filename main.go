package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/rjeczalik/notify"
)

const (
	Empty        = 0x0
	RunLevel     = 0x1
	BootTime     = 0x2
	NewTime      = 0x3
	OldTime      = 0x4
	InitProcess  = 0x5
	LoginProcess = 0x6
	UserProcess  = 0x7
	DeadProcess  = 0x8
	Accounting   = 0x9
)

const (
	LineSize = 32
	NameSize = 32
	HostSize = 256
)

// utmp structures
// see man utmp
type ExitStatus struct {
	Termination int16
	Exit        int16
}

type TimeVal struct {
	Sec  int32
	Usec int32
}

type Utmp struct {
	Type    int16
	Pid     int32
	Device  string
	Id      string
	User    string
	Host    string
	Exit    ExitStatus
	Session int32
	Time    time.Time
	Addr    net.IP
}

type Payload struct {
	f *os.File
	c chan notify.EventInfo
}

func (p Payload) read() (u *Utmp, err error) {
	type utmp struct {
		Type int16
		// alignment
		_       [2]byte
		Pid     int32
		Device  [LineSize]byte
		Id      [4]byte
		User    [NameSize]byte
		Host    [HostSize]byte
		Exit    ExitStatus
		Session int32
		Time    TimeVal
		AddrV6  [16]byte
		// Reserved member
		Reserved [20]byte
	}
	u_ := new(utmp)
	err = binary.Read(p.f, binary.LittleEndian, u_)
	if err != nil {
		return nil, err
	}
	toStr := func(arr []byte) string {
		return strings.Trim(string(arr), "\x00")
	}
	u = new(Utmp)
	u.Type = u_.Type
	u.Pid = u_.Pid
	u.Device = toStr(u_.Device[:])
	u.Id = toStr(u_.Id[:])
	u.User = toStr(u_.User[:])
	u.Host = toStr(u_.Host[:])
	u.Exit = u_.Exit
	u.Session = u_.Session
	u.Time = time.Unix(int64(u_.Time.Sec), int64(u_.Time.Usec)*1000)
	u.Addr = u_.AddrV6[:]
	return
}
func (p Payload) Read() (u *Utmp, err error) {
	<-p.c
	return p.read()
}
func NewPayload(file string) (*Payload, error) {
	c := make(chan notify.EventInfo, 1)
	err := notify.Watch(file, c, notify.Write)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	_, err = f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	return &Payload{f, c}, nil
}
func main() {
	input := flag.String("i", "", "utmp file input, like /var/log/wtmp")
	output := flag.String("o", "", "output file, if empty, output to stdout")
	flag.Parse()
	if *input == "" {
		fmt.Println("Need a input file")
		os.Exit(1)
	}
	out := os.Stdout
	if *output != "" {
		var err error
		out, err = os.Open(*output)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	var formater func(*Utmp) string = Default
	p, err := NewPayload(*input)
	if err != nil {
		panic(err)
	}
	for {
		u, err := p.Read()
		if err != nil {
			panic(err)
		}
		_, err = out.WriteString(formater(u))
		if err != nil {
			panic(err)
		}
	}
}
func Default(u *Utmp) string {
	t := u.Time.Format(time.DateTime)
	return fmt.Sprintf("%d,%d,%s,%s,%s,%s,%s", u.Type, u.Pid, u.Device, u.User, u.Id, u.Host, t)
}
