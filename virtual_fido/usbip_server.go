package virtual_fido

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
)

var usbipLogger = newLogger("[USBIP] ", false)

type usbIPServer struct {
	device        usbDevice
	responseMutex *sync.Mutex
}

func newUSBIPServer(device usbDevice) *usbIPServer {
	server := new(usbIPServer)
	server.device = device
	server.responseMutex = &sync.Mutex{}
	return server
}

func (server *usbIPServer) start() {
	usbipLogger.Println("Starting USBIP server...")
	listener, err := net.Listen("tcp", ":3240")
	checkErr(err, "Could not create listener")
	for {
		connection, err := listener.Accept()
		checkErr(err, "Connection accept error")
		if !strings.HasPrefix(connection.RemoteAddr().String(), "127.0.0.1") {
			usbipLogger.Printf("Connection attempted from non-local address: %s", connection.RemoteAddr().String())
			connection.Close()
			continue
		}
		server.handleConnection(&connection)
	}
}

func (server *usbIPServer) handleConnection(conn *net.Conn) {
	for {
		header := readBE[usbipControlHeader](*conn)
		usbipLogger.Printf("[CONTROL MESSAGE] %#v\n\n", header)
		if header.CommandCode == usbip_COMMAND_OP_REQ_DEVLIST {
			reply := newOpRepDevlist(server.device)
			usbipLogger.Printf("[OP_REP_DEVLIST] %#v\n\n", reply)
			write(*conn, toBE(reply))
		} else if header.CommandCode == usbip_COMMAND_OP_REQ_IMPORT {
			busId := make([]byte, 32)
			bytesRead, err := (*conn).Read(busId)
			if bytesRead != 32 {
				panic(fmt.Sprintf("Could not read busId for OP_REQ_IMPORT: %v", err))
			}
			reply := newOpRepImport(server.device)
			usbipLogger.Printf("[OP_REP_IMPORT] %s\n\n", reply)
			write(*conn, toBE(reply))
			server.handleCommands(conn)
		}
	}
}

func (server *usbIPServer) handleCommands(conn *net.Conn) {
	for {
		//fmt.Printf("--------------------------------------------\n\n")
		header := readBE[usbipMessageHeader](*conn)
		usbipLogger.Printf("[MESSAGE HEADER] %s\n\n", header)
		if header.Command == usbip_COMMAND_SUBMIT {
			server.handleCommandSubmit(conn, header)
		} else if header.Command == usbip_COMMAND_UNLINK {
			server.handleCommandUnlink(conn, header)
		} else {
			panic(fmt.Sprintf("Unsupported Command; %#v", header))
		}
	}
}

func (server *usbIPServer) handleCommandSubmit(conn *net.Conn, header usbipMessageHeader) {
	command := readBE[usbipCommandSubmitBody](*conn)
	setup := command.Setup()
	usbipLogger.Printf("[COMMAND SUBMIT] %s\n\n", command)
	transferBuffer := make([]byte, command.TransferBufferLength)
	if header.Direction == usbip_DIR_OUT && command.TransferBufferLength > 0 {
		_, err := (*conn).Read(transferBuffer)
		checkErr(err, "Could not read transfer buffer")
	}
	// Getting the reponse may not be immediate, so we need a callback
	onReturnSubmit := func() {
		server.responseMutex.Lock()
		replyHeader := usbipMessageHeader{
			Command:        usbip_COMMAND_RET_SUBMIT,
			SequenceNumber: header.SequenceNumber,
			DeviceId:       header.DeviceId,
			Direction:      usbip_DIR_OUT,
			Endpoint:       header.Endpoint,
		}
		replyBody := usbipReturnSubmitBody{
			Status:          0,
			ActualLength:    uint32(len(transferBuffer)),
			StartFrame:      0,
			NumberOfPackets: 0,
			ErrorCount:      0,
			Padding:         0,
		}
		usbipLogger.Printf("[RETURN SUBMIT] %v %#v\n\n", replyHeader, replyBody)
		write(*conn, toBE(replyHeader))
		write(*conn, toBE(replyBody))
		if header.Direction == usbip_DIR_IN {
			write(*conn, transferBuffer)
		}
		server.responseMutex.Unlock()
	}
	server.device.handleMessage(header.SequenceNumber, onReturnSubmit, header.Endpoint, setup, transferBuffer)
}

func (server *usbIPServer) handleCommandUnlink(conn *net.Conn, header usbipMessageHeader) {
	unlink := readBE[usbipCommandUnlinkBody](*conn)
	usbipLogger.Printf("[COMMAND UNLINK] %#v\n\n", unlink)
	var status int32
	if server.device.removeWaitingRequest(unlink.UnlinkSequenceNumber) {
		status = -int32(syscall.ECONNRESET)
	} else {
		status = -int32(syscall.ENOENT)
	}
	replyHeader := usbipMessageHeader{
		Command:        usbip_COMMAND_RET_UNLINK,
		SequenceNumber: header.SequenceNumber,
		DeviceId:       header.DeviceId,
		Direction:      usbip_DIR_OUT,
		Endpoint:       header.Endpoint,
	}
	replyBody := usbipReturnUnlinkBody{
		Status:  status,
		Padding: [24]byte{},
	}
	write(*conn, toBE(replyHeader))
	write(*conn, toBE(replyBody))
}
