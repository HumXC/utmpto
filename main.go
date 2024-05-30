package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
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

// utmp structures
// see man utmp
type ExitStatus struct {
	Termination int16 `json:"termination"`
	Exit        int16 `json:"exit"`
}

type Utmp struct {
	Type    int16      `json:"type"`
	Pid     int32      `json:"pid"`
	Device  string     `json:"device"`
	Id      string     `json:"id"`
	User    string     `json:"user"`
	Host    string     `json:"host"`
	Exit    ExitStatus `json:"exit_status"`
	Session int32      `json:"session"`
	Time    Time       `json:"time"`
	Addr    net.IP     `json:"addr"`
}

type Payload struct {
	f *os.File
	c chan notify.EventInfo
}
type Time struct {
	time.Time
	Layout string
}

func (t Time) MarshalJSON() ([]byte, error) {
	if t.Layout == "" {
		return nil, errors.New("Time.MarshalJSON: layout is empty")
	}
	s := t.Format(t.Layout)
	return []byte(("\"" + s + "\"")), nil
}
func (p Payload) read() (u *Utmp, err error) {
	type utmp struct {
		Type int16
		// alignment
		_       [2]byte
		Pid     int32
		Device  [32]byte
		Id      [4]byte
		User    [32]byte
		Host    [256]byte
		Exit    ExitStatus
		Session int32
		Time    struct {
			Sec  int32
			Usec int32
		}
		AddrV6 [16]byte
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
	u.Time = Time{
		Time:   time.Unix(int64(u_.Time.Sec), int64(u_.Time.Usec)*1000),
		Layout: time.DateTime,
	}
	ip := make(net.IP, 16)
	_ = binary.Read(bytes.NewReader(u_.AddrV6[:]), binary.BigEndian, ip)
	if ip[4:].Equal(net.IPv6zero[4:]) {
		ip = ip[:4]
	}
	u.Addr = ip
	return
}
func (p Payload) Read() (u *Utmp, err error) {
	<-p.c
	return p.read()
}
func NewPayload(file string, isSeekEnd bool) (*Payload, error) {
	c := make(chan notify.EventInfo, 1)
	err := notify.Watch(file, c, notify.Write)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	if isSeekEnd {
		_, err = f.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
	}
	return &Payload{f, c}, nil
}
func main() {
	input := flag.String("i", "", "utmp file input, like /var/log/wtmp")
	output := flag.String("o", "", "output file, if empty, output to stdout")
	isStartFileBeginning := flag.Bool("s", false, "Start with the file at the beginning.")

	flag.Parse()
	if *input == "" {
		fmt.Println("Need a input file")
		os.Exit(1)
	}
	out := os.Stdout
	if *output != "" {
		var err error
		out, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	var formater func(*Utmp) string = Json
	p, err := NewPayload(*input, !*isStartFileBeginning)
	if err != nil {
		panic(err)
	}
	for *isStartFileBeginning {
		u, err := p.read()
		if err != nil {
			break
		}
		_, err = out.WriteString(formater(u) + "\n")
		if err != nil {
			panic(err)
		}
	}
	for {
		u, err := p.Read()
		if err != nil {
			panic(err)
		}
		_, err = out.WriteString(formater(u) + "\n")
		if err != nil {
			panic(err)
		}
	}
}
func Json(u *Utmp) string {
	b, err := json.Marshal(u)
	if err != nil {
		panic(err)
	}
	return string(b)
}
